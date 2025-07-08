// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
)

// AttendanceQueryRequest represents the structured query request
type AttendanceQueryRequest struct {
	Type       string   `json:"type"`   // "self", "by_names", "all_employees"
	Names      []string `json:"names"`  // Empty for self/all_employees, contains names for by_names
	Status     string   `json:"status"` // "Present", "Absent", etc.
	TimePeriod struct {
		Type        string `json:"type"`
		StartDate   string `json:"start_date"`
		EndDate     string `json:"end_date"`
		Description string `json:"description"`
	} `json:"time_period"`
}

// handleAttendanceReportRequest processes unified attendance report requests
func (m *AttendanceModule) handleAttendanceReportRequest(ctx *erp_modules.ModuleContext, intent *erp_modules.Intent) (*erp_modules.ModuleResponse, error) {
	isVietnamese := detectUserLanguage(ctx.User)

	// Use LLM to analyze the user's query and extract structured request
	queryRequest, err := m.analyzeAttendanceQuery(ctx, intent.RawMessage)
	if err != nil {
		errorMsg := "⚠️ Không thể hiểu được yêu cầu của bạn. Vui lòng thử lại với câu hỏi rõ ràng hơn."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot understand your request. Please try again with a clearer question."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Handle based on query type
	switch queryRequest.Type {
	case "self":
		return m.handleSelfAttendanceReport(ctx, queryRequest, isVietnamese)
	case "by_names":
		return m.handleNameBasedReport(ctx, queryRequest, isVietnamese)
	case "all_employees":
		return m.handleAllEmployeesReport(ctx, queryRequest, isVietnamese)
	default:
		errorMsg := "⚠️ Loại báo cáo không được hỗ trợ."
		if !isVietnamese {
			errorMsg = "⚠️ Unsupported report type."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}
}

// handleSelfAttendanceReport handles attendance queries for the current user
func (m *AttendanceModule) handleSelfAttendanceReport(ctx *erp_modules.ModuleContext, queryRequest *AttendanceQueryRequest, isVietnamese bool) (*erp_modules.ModuleResponse, error) {
	// Get employee ID for the current user
	employeeID, err := m.getEmployeeIDFromUser(ctx.User)
	if err != nil {
		errorMsg := "Không tìm thấy thông tin nhân viên của bạn trong hệ thống ERP. Vui lòng liên hệ quản trị viên."
		if !isVietnamese {
			errorMsg = "Cannot find your employee information in the ERP system. Please contact administrator."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	// Generate attendance report for the current user
	report, err := m.erpClient.GetAttendanceForEmployee(
		employeeID,
		queryRequest.TimePeriod.StartDate,
		queryRequest.TimePeriod.EndDate,
	)
	if err != nil {
		errorMsg := "⚠️ Có lỗi xảy ra khi truy vấn dữ liệu chấm công. Vui lòng thử lại."
		if !isVietnamese {
			errorMsg = "⚠️ An error occurred while querying attendance data. Please try again."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	report.EmployeeName = getUserDisplayName(ctx.User)
	reports := []*EmployeeAttendanceReport{report}

	// Generate formatted response using the same table format as other reports
	responseMessage := formatAttendanceReports(reports, queryRequest, isVietnamese, "self")

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     responseMessage,
		ActionTaken: "get_attendance_report",
		Data: map[string]interface{}{
			"type":        "self",
			"employee_id": employeeID,
			"time_period": queryRequest.TimePeriod.Description,
		},
	}, nil
}

// handleNameBasedReport generates report for specific employees by name
func (m *AttendanceModule) handleNameBasedReport(ctx *erp_modules.ModuleContext, queryRequest *AttendanceQueryRequest, isVietnamese bool) (*erp_modules.ModuleResponse, error) {
	var allReports []*EmployeeAttendanceReport
	var allMatchedEmployees []Employee

	// Search for employees matching each name
	for _, name := range queryRequest.Names {
		matchedEmployees, err := m.erpClient.SearchEmployeesByName(name)
		if err != nil {
			m.api.LogError("Failed to search employees by name", "name", name, "error", err.Error())
			continue
		}
		allMatchedEmployees = append(allMatchedEmployees, matchedEmployees...)
	}

	// Remove duplicates (same employee matched by different names)
	uniqueEmployees := removeDuplicateEmployees(allMatchedEmployees)

	if len(uniqueEmployees) == 0 {
		errorMsg := fmt.Sprintf("Không tìm thấy nhân viên nào phù hợp với tên: %s", strings.Join(queryRequest.Names, ", "))
		if !isVietnamese {
			errorMsg = fmt.Sprintf("No employees found matching names: %s", strings.Join(queryRequest.Names, ", "))
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	// Limit results to prevent overwhelming reports
	maxResults := 20
	if len(uniqueEmployees) > maxResults {
		uniqueEmployees = uniqueEmployees[:maxResults]
	}

	// Generate attendance reports for each matched employee
	for _, employee := range uniqueEmployees {
		report, err := m.erpClient.GetAttendanceForEmployee(
			employee.Name,
			queryRequest.TimePeriod.StartDate,
			queryRequest.TimePeriod.EndDate,
		)
		if err != nil {
			// Create error report for failed employee
			report = &EmployeeAttendanceReport{
				EmployeeID:   employee.Name,
				EmployeeName: employee.EmployeeName,
				ErrorMessage: err.Error(),
			}
		} else {
			report.EmployeeName = employee.EmployeeName
		}
		allReports = append(allReports, report)
	}

	// Generate formatted response
	responseMessage := formatAttendanceReports(allReports, queryRequest, isVietnamese, "by_names")

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     responseMessage,
		ActionTaken: "get_attendance_report",
		Data: map[string]interface{}{
			"type":              "by_names",
			"searched_names":    queryRequest.Names,
			"employees_found":   len(uniqueEmployees),
			"reports_generated": len(allReports),
			"time_period":       queryRequest.TimePeriod.Description,
		},
	}, nil
}

// handleAllEmployeesReport generates report for all employees
func (m *AttendanceModule) handleAllEmployeesReport(ctx *erp_modules.ModuleContext, queryRequest *AttendanceQueryRequest, isVietnamese bool) (*erp_modules.ModuleResponse, error) {
	// Get all employees from ERP
	allEmployees, err := m.erpClient.GetAllEmployees()
	if err != nil {
		errorMsg := "⚠️ Không thể lấy danh sách nhân viên từ hệ thống ERP."
		if !isVietnamese {
			errorMsg = "⚠️ Cannot retrieve employee list from ERP system."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
			Error:   err.Error(),
		}, nil
	}

	if len(allEmployees) == 0 {
		errorMsg := "Không có nhân viên nào trong hệ thống."
		if !isVietnamese {
			errorMsg = "No employees found in the system."
		}
		return &erp_modules.ModuleResponse{
			Success: false,
			Message: errorMsg,
		}, nil
	}

	var allReports []*EmployeeAttendanceReport

	// Generate attendance reports for all employees
	successCount := 0
	for _, employee := range allEmployees {
		report, err := m.erpClient.GetAttendanceForEmployee(
			employee.Name,
			queryRequest.TimePeriod.StartDate,
			queryRequest.TimePeriod.EndDate,
		)
		if err != nil {
			// Create error report for failed employee
			report = &EmployeeAttendanceReport{
				EmployeeID:   employee.Name,
				EmployeeName: employee.EmployeeName,
				ErrorMessage: err.Error(),
			}
		} else {
			report.EmployeeName = employee.EmployeeName
			successCount++
		}
		allReports = append(allReports, report)
	}

	// Generate formatted response
	responseMessage := formatAttendanceReports(allReports, queryRequest, isVietnamese, "all_employees")

	return &erp_modules.ModuleResponse{
		Success:     true,
		Message:     responseMessage,
		ActionTaken: "get_attendance_report",
		Data: map[string]interface{}{
			"type":               "all_employees",
			"total_employees":    len(allEmployees),
			"successful_reports": successCount,
			"failed_reports":     len(allEmployees) - successCount,
			"time_period":        queryRequest.TimePeriod.Description,
		},
	}, nil
}

// formatAttendanceReports formats the attendance reports into a readable table message
func formatAttendanceReports(reports []*EmployeeAttendanceReport, request *AttendanceQueryRequest, isVietnamese bool, reportType string) string {
	var message strings.Builder

	// Header based on report type
	switch reportType {
	case "self":
		if isVietnamese {
			message.WriteString(fmt.Sprintf("**Báo cáo chấm công của bạn** (%s)\n\n", request.TimePeriod.Description))
		} else {
			message.WriteString(fmt.Sprintf("**Your Attendance Report** (%s)\n\n", request.TimePeriod.Description))
		}
	case "all_employees":
		if isVietnamese {
			message.WriteString(fmt.Sprintf("**Báo cáo chấm công tất cả nhân viên** (%s)\n\n", request.TimePeriod.Description))
		} else {
			message.WriteString(fmt.Sprintf("**All Employees Attendance Report** (%s)\n\n", request.TimePeriod.Description))
		}
	case "by_names":
		searchedNames := strings.Join(request.Names, ", ")
		if isVietnamese {
			message.WriteString(fmt.Sprintf("**Báo cáo chấm công cho '%s'** (%s)\n\n", searchedNames, request.TimePeriod.Description))
		} else {
			message.WriteString(fmt.Sprintf("**Attendance Report for '%s'** (%s)\n\n", searchedNames, request.TimePeriod.Description))
		}
	}

	// Check if we have any reports
	if len(reports) == 0 {
		if isVietnamese {
			message.WriteString("Không có dữ liệu chấm công.")
		} else {
			message.WriteString("No attendance data available.")
		}
		return message.String()
	}

	// Table headers
	if isVietnamese {
		message.WriteString("| **Nhân viên** | **Tổng** | **Có mặt** | **Vắng** |\n")
	} else {
		message.WriteString("| **Employee** | **Total** | **Present** | **Absent** |\n")
	}
	message.WriteString("|---|---:|---:|---:|\n")

	// Limit display to prevent overwhelming messages
	displayLimit := 50
	displayedCount := 0

	// Table rows
	for _, report := range reports {
		if displayedCount >= displayLimit {
			remaining := len(reports) - displayedCount
			if isVietnamese {
				message.WriteString(fmt.Sprintf("| *...và %d nhân viên khác* | | | |\n", remaining))
			} else {
				message.WriteString(fmt.Sprintf("| *...and %d more employees* | | | |\n", remaining))
			}
			break
		}

		if report.ErrorMessage != "" {
			// Show error row
			if isVietnamese {
				message.WriteString(fmt.Sprintf("| %s | ❌ | ❌ | ❌ |\n", report.EmployeeName))
			} else {
				message.WriteString(fmt.Sprintf("| %s | ❌ | ❌ | ❌ |\n", report.EmployeeName))
			}
		} else {
			// Show data row
			message.WriteString(fmt.Sprintf("| %s | %d | %d | %d |\n",
				report.EmployeeName,
				report.TotalDays,
				report.PresentDays,
				report.AbsentDays))
		}

		displayedCount++
	}

	// Add note about using more specific names if results were limited
	if displayedCount >= displayLimit {
		if isVietnamese {
			message.WriteString("\n💡 *Sử dụng tên cụ thể hơn để thu hẹp kết quả.*")
		} else {
			message.WriteString("\n💡 *Use more specific names to narrow results.*")
		}
	}

	return message.String()
}
