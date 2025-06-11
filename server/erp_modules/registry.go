// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package erp_modules

import (
	"fmt"
	"sync"
)

// Registry implements ModuleRegistry interface
type Registry struct {
	modules map[string]ERPModule
	mutex   sync.RWMutex
}

// NewRegistry creates a new module registry
func NewRegistry() *Registry {
	return &Registry{
		modules: make(map[string]ERPModule),
	}
}

// RegisterModule registers a new ERP module
func (r *Registry) RegisterModule(module ERPModule) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	category := module.GetCategory()
	if category == "" {
		return fmt.Errorf("module category cannot be empty")
	}

	if _, exists := r.modules[category]; exists {
		return fmt.Errorf("module for category '%s' already exists", category)
	}

	r.modules[category] = module
	return nil
}

// GetModule retrieves a module by category
func (r *Registry) GetModule(category string) (ERPModule, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	module, exists := r.modules[category]
	return module, exists
}

// GetAllModules returns all registered modules
func (r *Registry) GetAllModules() []ERPModule {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	modules := make([]ERPModule, 0, len(r.modules))
	for _, module := range r.modules {
		modules = append(modules, module)
	}
	return modules
}

// RouteIntent finds the appropriate module for the given intent
func (r *Registry) RouteIntent(intent *Intent) (ERPModule, error) {
	if intent == nil {
		return nil, fmt.Errorf("intent cannot be nil")
	}

	// First try to get module by category
	if module, exists := r.GetModule(intent.Category); exists {
		if module.CanHandle(intent) {
			return module, nil
		}
	}

	// If no direct match, check all modules
	for _, module := range r.GetAllModules() {
		if module.CanHandle(intent) {
			return module, nil
		}
	}

	return nil, fmt.Errorf("no module found to handle intent category '%s' with action '%s'",
		intent.Category, intent.Action)
}
