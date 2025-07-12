// Enhanced search.go for attendance module with improved fuzzy matching algorithms
// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package attendance

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// SearchEmployeesByName searches for employees by name using enhanced fuzzy matching
func (c *ERPClient) SearchEmployeesByName(searchName string) ([]Employee, error) {
	c.api.LogDebug("Searching employees by name", "search_name", searchName)

	// Get all employees first
	allEmployees, err := c.GetAllEmployees()
	if err != nil {
		return nil, err
	}

	// Filter employees using enhanced fuzzy matching
	var matchedEmployees []Employee
	searchNameNormalized := normalizeText(searchName)

	for _, employee := range allEmployees {
		// Check multiple fields for matches
		employeeNameNormalized := normalizeText(employee.EmployeeName)
		employeeIDLower := strings.ToLower(employee.Name)
		employeeNumberLower := strings.ToLower(employee.EmployeeNumber)

		// Calculate enhanced confidence score
		confidence := calculateNameMatchConfidence(searchNameNormalized, employeeNameNormalized, employeeIDLower, employeeNumberLower)

		// Use lower threshold for better recall and more user choices
		if confidence >= 0.4 { // Further lowered to give users more relevant options
			employee.MatchConfidence = confidence
			matchedEmployees = append(matchedEmployees, employee)
		}
	}

	// Sort by confidence (highest first)
	sort.Slice(matchedEmployees, func(i, j int) bool {
		return matchedEmployees[i].MatchConfidence > matchedEmployees[j].MatchConfidence
	})

	// Increase results limit to give users more choices
	if len(matchedEmployees) > 25 {
		matchedEmployees = matchedEmployees[:25]
	}

	c.api.LogDebug("Found matching employees", "count", len(matchedEmployees), "search_name", searchName)

	return matchedEmployees, nil
}

// removeDuplicateEmployees removes duplicate employees from the list
func removeDuplicateEmployees(employees []Employee) []Employee {
	seen := make(map[string]bool)
	var unique []Employee

	for _, emp := range employees {
		if !seen[emp.Name] {
			seen[emp.Name] = true
			unique = append(unique, emp)
		}
	}

	return unique
}

// normalizeText normalizes text for better matching (handles Vietnamese characters, etc.)
func normalizeText(text string) string {
	// Convert to lowercase
	text = strings.ToLower(text)

	// Remove extra spaces and trim
	text = strings.TrimSpace(text)
	text = strings.Join(strings.Fields(text), " ")

	// Vietnamese character normalization mapping
	vietnameseMap := map[string]string{
		"à": "a", "á": "a", "ạ": "a", "ả": "a", "ã": "a",
		"â": "a", "ầ": "a", "ấ": "a", "ậ": "a", "ẩ": "a", "ẫ": "a",
		"ă": "a", "ằ": "a", "ắ": "a", "ặ": "a", "ẳ": "a", "ẵ": "a",
		"è": "e", "é": "e", "ẹ": "e", "ẻ": "e", "ẽ": "e",
		"ê": "e", "ề": "e", "ế": "e", "ệ": "e", "ể": "e", "ễ": "e",
		"ì": "i", "í": "i", "ị": "i", "ỉ": "i", "ĩ": "i",
		"ò": "o", "ó": "o", "ọ": "o", "ỏ": "o", "õ": "o",
		"ô": "o", "ồ": "o", "ố": "o", "ộ": "o", "ổ": "o", "ỗ": "o",
		"ơ": "o", "ờ": "o", "ớ": "o", "ợ": "o", "ở": "o", "ỡ": "o",
		"ù": "u", "ú": "u", "ụ": "u", "ủ": "u", "ũ": "u",
		"ư": "u", "ừ": "u", "ứ": "u", "ự": "u", "ử": "u", "ữ": "u",
		"ỳ": "y", "ý": "y", "ỵ": "y", "ỷ": "y", "ỹ": "y",
		"đ": "d",
	}

	// Apply Vietnamese normalization
	for vietnamese, latin := range vietnameseMap {
		text = strings.ReplaceAll(text, vietnamese, latin)
	}

	return text
}

// calculateNameMatchConfidence calculates name matching confidence with enhanced algorithm
func calculateNameMatchConfidence(searchName, employeeName, employeeID, employeeNumber string) float64 {
	var maxConfidence float64

	// Early exit for empty inputs
	if searchName == "" || employeeName == "" {
		return 0.0
	}

	// 1. Exact match (highest priority)
	if searchName == employeeName {
		return 1.0
	}

	// 2. Case-insensitive exact match
	if strings.EqualFold(searchName, employeeName) {
		maxConfidence = math.Max(maxConfidence, 0.98)
	}

	// 3. Enhanced substring matching with better relevance scoring
	if strings.Contains(employeeName, searchName) {
		coverage := float64(len(searchName)) / float64(len(employeeName))
		// Prioritize matches where search term covers more of the name
		substringScore := 0.75 + (coverage * 0.2) // 0.75 - 0.95
		maxConfidence = math.Max(maxConfidence, substringScore)
	}

	if strings.Contains(searchName, employeeName) {
		// When the search contains the full employee name
		maxConfidence = math.Max(maxConfidence, 0.82)
	}

	// 4. Word-based matching (critical for names like "Nguyen Van An")
	wordScore := calculateWordBasedScore(searchName, employeeName)
	maxConfidence = math.Max(maxConfidence, wordScore)

	// 5. Enhanced name-specific matching with Vietnamese patterns
	nameScore := calculateNameSpecificScore(searchName, employeeName)
	maxConfidence = math.Max(maxConfidence, nameScore)

	// 6. Partial word matching (important for Vietnamese names)
	partialWordScore := calculatePartialWordMatching(searchName, employeeName)
	maxConfidence = math.Max(maxConfidence, partialWordScore)

	// 7. N-gram matching with adjusted weight
	ngramScore := calculateNGramScore(searchName, employeeName, 3)
	maxConfidence = math.Max(maxConfidence, ngramScore*0.65) // Reduced weight for n-grams

	// 8. Enhanced fuzzy matching
	if len(searchName) > 2 && len(employeeName) > 2 {
		fuzzyScore := calculateEnhancedFuzzyMatch(searchName, employeeName)
		maxConfidence = math.Max(maxConfidence, fuzzyScore)
	}

	// 9. Employee ID matching (high confidence but not too high)
	if employeeID != "" && strings.Contains(employeeID, searchName) {
		maxConfidence = math.Max(maxConfidence, 0.90)
	}

	// 10. Employee number matching (high confidence but not too high)
	if employeeNumber != "" && strings.Contains(employeeNumber, searchName) {
		maxConfidence = math.Max(maxConfidence, 0.90)
	}

	// 11. Initials matching with moderate confidence
	initialsScore := calculateInitialsScore(searchName, employeeName)
	maxConfidence = math.Max(maxConfidence, initialsScore)

	// 12. Phonetic similarity for Vietnamese names
	phoneticScore := calculateVietnamesePhoneticSimilarity(searchName, employeeName)
	maxConfidence = math.Max(maxConfidence, phoneticScore)

	return maxConfidence
}

// calculateWordBasedScore calculates matching score based on word overlap
func calculateWordBasedScore(searchName, targetName string) float64 {
	searchWords := strings.Fields(searchName)
	targetWords := strings.Fields(targetName)

	if len(searchWords) == 0 || len(targetWords) == 0 {
		return 0.0
	}

	var totalMatches float64
	var partialMatches float64

	for _, searchWord := range searchWords {
		bestWordMatch := 0.0

		for _, targetWord := range targetWords {
			// Exact word match
			if searchWord == targetWord {
				bestWordMatch = 1.0
				break
			}

			// Substring match within words
			if strings.Contains(targetWord, searchWord) && len(searchWord) >= 2 {
				coverage := float64(len(searchWord)) / float64(len(targetWord))
				wordScore := 0.7 + (coverage * 0.25) // 0.7 - 0.95
				bestWordMatch = math.Max(bestWordMatch, wordScore)
			}

			if strings.Contains(searchWord, targetWord) && len(targetWord) >= 2 {
				bestWordMatch = math.Max(bestWordMatch, 0.8)
			}

			// Fuzzy match within words (for typos)
			if len(searchWord) > 2 && len(targetWord) > 2 {
				wordFuzzy := calculateSimpleEditDistance(searchWord, targetWord)
				if wordFuzzy >= 0.7 {
					bestWordMatch = math.Max(bestWordMatch, wordFuzzy*0.85)
				}
			}
		}

		if bestWordMatch >= 0.5 { // Lowered threshold for word matches
			totalMatches += bestWordMatch
		} else if bestWordMatch > 0.0 {
			partialMatches += bestWordMatch
		}
	}

	// Calculate final score
	fullMatchScore := totalMatches / float64(len(searchWords))
	partialMatchScore := partialMatches / float64(len(searchWords))

	// Combine scores with weights favoring full matches
	combinedScore := (fullMatchScore * 0.8) + (partialMatchScore * 0.2)

	// Boost score if reasonable number of words matched
	if totalMatches >= float64(len(searchWords))*0.5 { // Lowered from 0.7
		combinedScore *= 0.85 // More generous scaling
	} else {
		combinedScore *= 0.75 // Still reasonable for partial matches
	}

	return combinedScore
}

// calculateNameSpecificScore handles name-specific matching patterns
func calculateNameSpecificScore(searchName, fullName string) float64 {
	searchWords := strings.Fields(searchName)
	nameWords := strings.Fields(fullName)

	if len(searchWords) == 0 || len(nameWords) == 0 {
		return 0.0
	}

	// Handle common Vietnamese name patterns
	if len(nameWords) >= 2 {
		firstName := nameWords[len(nameWords)-1] // Last word is usually first name in Vietnamese
		lastName := nameWords[0]                 // First word is usually family name

		// Check if search matches first name or last name exactly
		for _, searchWord := range searchWords {
			if searchWord == firstName {
				return 0.80 // Good confidence for first name match
			}
			if searchWord == lastName {
				return 0.70 // Good confidence for family name match
			}
		}

		// Check if search is combination of first + family name
		if len(searchWords) == 2 {
			if (searchWords[0] == lastName && searchWords[1] == firstName) ||
				(searchWords[0] == firstName && searchWords[1] == lastName) {
				return 0.85 // High confidence for full name match
			}
		}

		// Check for partial matches in middle names or other components
		for _, searchWord := range searchWords {
			for _, nameWord := range nameWords {
				if len(nameWord) > 2 && strings.Contains(nameWord, searchWord) {
					return 0.65 // Moderate confidence for partial component match
				}
			}
		}
	}

	return 0.0
}

// calculatePartialWordMatching handles partial matching within words (important for Vietnamese)
func calculatePartialWordMatching(searchName, targetName string) float64 {
	searchWords := strings.Fields(searchName)
	targetWords := strings.Fields(targetName)

	if len(searchWords) == 0 || len(targetWords) == 0 {
		return 0.0
	}

	var bestScore float64

	for _, searchWord := range searchWords {
		if len(searchWord) < 2 {
			continue
		}

		for _, targetWord := range targetWords {
			if len(targetWord) < 2 {
				continue
			}

			// Check if search word is a significant part of target word
			if strings.Contains(targetWord, searchWord) {
				coverage := float64(len(searchWord)) / float64(len(targetWord))
				if coverage >= 0.5 { // At least half the word
					score := 0.5 + (coverage * 0.3) // 0.5 - 0.8
					bestScore = math.Max(bestScore, score)
				}
			}

			// Check reverse - if target word is part of search word
			if strings.Contains(searchWord, targetWord) {
				coverage := float64(len(targetWord)) / float64(len(searchWord))
				if coverage >= 0.5 {
					score := 0.55 + (coverage * 0.25) // 0.55 - 0.8
					bestScore = math.Max(bestScore, score)
				}
			}
		}
	}

	return bestScore
}

// calculateVietnamesePhoneticSimilarity handles Vietnamese phonetic similarities
func calculateVietnamesePhoneticSimilarity(searchName, targetName string) float64 {
	// Common Vietnamese phonetic substitutions
	phoneticMap := map[string][]string{
		"ph": {"f"},
		"th": {"t"},
		"tr": {"ch"},
		"gi": {"d", "z"},
		"qu": {"kw", "k"},
		"ng": {"n"},
		"nh": {"n"},
		"ch": {"tr", "c"},
		"kh": {"k"},
		"gh": {"g"},
		"x":  {"s"},
		"d":  {"gi", "z"},
		"v":  {"w"},
	}

	searchNormalized := normalizeForPhonetic(searchName)
	targetNormalized := normalizeForPhonetic(targetName)

	// Try phonetic substitutions
	maxSimilarity := 0.0

	// Generate phonetic variants for search term
	searchVariants := generatePhoneticVariants(searchNormalized, phoneticMap)
	targetVariants := generatePhoneticVariants(targetNormalized, phoneticMap)

	// Compare all variants
	for _, searchVariant := range searchVariants {
		for _, targetVariant := range targetVariants {
			if strings.Contains(targetVariant, searchVariant) {
				coverage := float64(len(searchVariant)) / float64(len(targetVariant))
				similarity := 0.4 + (coverage * 0.2) // 0.4 - 0.6
				maxSimilarity = math.Max(maxSimilarity, similarity)
			}
		}
	}

	return maxSimilarity
}

// normalizeForPhonetic normalizes text for phonetic comparison
func normalizeForPhonetic(text string) string {
	text = normalizeText(text) // Apply existing normalization first

	// Additional phonetic normalizations
	text = strings.ReplaceAll(text, "oo", "u")
	text = strings.ReplaceAll(text, "ee", "i")
	text = strings.ReplaceAll(text, "aa", "a")

	return text
}

// generatePhoneticVariants generates phonetic variants of a text
func generatePhoneticVariants(text string, phoneticMap map[string][]string) []string {
	variants := []string{text}

	for original, substitutes := range phoneticMap {
		var newVariants []string
		for _, variant := range variants {
			newVariants = append(newVariants, variant)
			if strings.Contains(variant, original) {
				for _, substitute := range substitutes {
					newVariant := strings.ReplaceAll(variant, original, substitute)
					newVariants = append(newVariants, newVariant)
				}
			}
		}
		variants = newVariants
	}

	// Remove duplicates and return first 10 variants to avoid explosion
	seen := make(map[string]bool)
	var uniqueVariants []string
	for _, variant := range variants {
		if !seen[variant] && len(uniqueVariants) < 10 {
			seen[variant] = true
			uniqueVariants = append(uniqueVariants, variant)
		}
	}

	return uniqueVariants
}

// calculateNGramScore calculates similarity using n-gram analysis
func calculateNGramScore(s1, s2 string, n int) float64 {
	if len(s1) < n || len(s2) < n {
		return 0.0
	}

	ngrams1 := generateNGrams(s1, n)
	ngrams2 := generateNGrams(s2, n)

	if len(ngrams1) == 0 || len(ngrams2) == 0 {
		return 0.0
	}

	intersection := 0
	ngrams2Set := make(map[string]bool)
	for _, ngram := range ngrams2 {
		ngrams2Set[ngram] = true
	}

	for _, ngram := range ngrams1 {
		if ngrams2Set[ngram] {
			intersection++
		}
	}

	// Jaccard similarity
	union := len(ngrams1) + len(ngrams2) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// generateNGrams generates n-grams from a string
func generateNGrams(s string, n int) []string {
	if len(s) < n {
		return []string{}
	}

	var ngrams []string
	for i := 0; i <= len(s)-n; i++ {
		ngrams = append(ngrams, s[i:i+n])
	}
	return ngrams
}

// calculateInitialsScore checks if search matches initials
func calculateInitialsScore(searchName, fullName string) float64 {
	words := strings.Fields(fullName)
	if len(words) <= 1 {
		return 0.0
	}

	var initials strings.Builder
	for _, word := range words {
		if len(word) > 0 {
			initials.WriteByte(byte(unicode.ToLower(rune(word[0]))))
		}
	}

	initialsStr := initials.String()
	if len(initialsStr) > 0 && strings.Contains(initialsStr, strings.ToLower(searchName)) {
		return 0.6 // Moderate confidence for initials match
	}

	// Also check if searchName matches initials pattern (e.g., "nva" for "Nguyen Van An")
	searchLower := strings.ToLower(searchName)
	if len(searchLower) >= 2 && len(searchLower) <= len(initialsStr) {
		if strings.HasPrefix(initialsStr, searchLower) {
			return 0.65 // Good confidence for prefix initials match
		}
	}

	return 0.0
}

// calculateEnhancedFuzzyMatch provides better fuzzy matching than simple Levenshtein
func calculateEnhancedFuzzyMatch(s1, s2 string) float64 {
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	// Use Damerau-Levenshtein distance (allows transpositions)
	distance := calculateDamerauLevenshteinDistance(s1, s2)
	maxLen := maxInt(len(s1), len(s2))

	if maxLen == 0 {
		return 1.0
	}

	similarity := 1.0 - float64(distance)/float64(maxLen)

	// Apply more generous thresholds for fuzzy matches
	if similarity >= 0.80 {
		return similarity * 0.85 // Good fuzzy match
	} else if similarity >= 0.70 {
		return similarity * 0.75 // Moderate fuzzy match
	} else if similarity >= 0.60 {
		return similarity * 0.65 // Low but acceptable fuzzy match
	} else if similarity >= 0.45 {
		return similarity * 0.50 // Very low but still relevant
	}

	return 0.0 // Too dissimilar
}

// calculateSimpleEditDistance calculates edit distance similarity (for word-level matching)
func calculateSimpleEditDistance(s1, s2 string) float64 {
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	distance := calculateLevenshteinDistance(s1, s2)
	maxLen := maxInt(len(s1), len(s2))

	if maxLen == 0 {
		return 1.0
	}

	return 1.0 - float64(distance)/float64(maxLen)
}

// calculateLevenshteinDistance calculates Levenshtein distance
func calculateLevenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create matrix
	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
	}

	// Initialize first row and column
	for i := 0; i <= len(s1); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}

			matrix[i][j] = minInt3(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

// calculateDamerauLevenshteinDistance calculates Damerau-Levenshtein distance (includes transpositions)
func calculateDamerauLevenshteinDistance(s1, s2 string) int {
	len1, len2 := len(s1), len(s2)

	if len1 == 0 {
		return len2
	}
	if len2 == 0 {
		return len1
	}

	// Create matrix
	h := make([][]int, len1+2)
	for i := range h {
		h[i] = make([]int, len2+2)
	}

	maxdist := len1 + len2
	h[0][0] = maxdist

	// Initialize first row and column
	for i := 0; i <= len1; i++ {
		h[i+1][0] = maxdist
		h[i+1][1] = i
	}
	for j := 0; j <= len2; j++ {
		h[0][j+1] = maxdist
		h[1][j+1] = j
	}

	// Character frequency map
	charMap := make(map[rune]int)
	for _, char := range s1 + s2 {
		charMap[char] = 0
	}

	for i := 1; i <= len1; i++ {
		db := 0
		for j := 1; j <= len2; j++ {
			k := charMap[rune(s2[j-1])]
			l := db
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
				db = j
			}

			h[i+1][j+1] = minInt4(
				h[i][j]+cost,              // substitution
				h[i+1][j]+1,               // insertion
				h[i][j+1]+1,               // deletion
				h[k][l]+(i-k-1)+1+(j-l-1), // transposition
			)
		}
		charMap[rune(s1[i-1])] = i
	}

	return h[len1+1][len2+1]
}

// calculateFuzzyMatch - kept for compatibility but now calls enhanced version
func calculateFuzzyMatch(s1, s2 string) float64 {
	return calculateEnhancedFuzzyMatch(s1, s2)
}

// calculateNameSimilarity - enhanced version of the existing function
func calculateNameSimilarity(name1, name2 string) float64 {
	name1 = normalizeText(name1)
	name2 = normalizeText(name2)

	if name1 == name2 {
		return 1.0
	}

	// Use the enhanced matching algorithm
	return calculateNameMatchConfidence(name1, name2, "", "")
}
