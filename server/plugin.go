// Copyright (c) 2023-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"

	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/mattermost/mattermost-plugin-ai/server/anthropic"
	"github.com/mattermost/mattermost-plugin-ai/server/embeddings"
	"github.com/mattermost/mattermost-plugin-ai/server/enterprise"
	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules"
	"github.com/mattermost/mattermost-plugin-ai/server/erp_modules/attendance"
	"github.com/mattermost/mattermost-plugin-ai/server/llm"
	"github.com/mattermost/mattermost-plugin-ai/server/metrics"
	"github.com/mattermost/mattermost-plugin-ai/server/openai"
	"github.com/mattermost/mattermost-plugin-ai/server/postgres"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/shared/httpservice"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

const (
	BotUsername = "ai"

	CallsRecordingPostType = "custom_calls_recording"
	CallsBotUsername       = "calls"
	ZoomBotUsername        = "zoom"

	ffmpegPluginPath = "./plugins/mattermost-ai/server/dist/ffmpeg"
)

//go:embed llm/prompts
var promptsFolder embed.FS

// Plugin implements the interface expected by the Mattermost server to communicate between the server and plugin processes.
type Plugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	pluginAPI *pluginapi.Client

	ffmpegPath string

	db      *sqlx.DB
	builder sq.StatementBuilderType

	prompts *llm.Prompts

	streamingContexts      map[string]PostStreamContext
	streamingContextsMutex sync.Mutex

	licenseChecker *enterprise.LicenseChecker
	metricsService metrics.Metrics
	metricsHandler http.Handler

	botsLock sync.RWMutex
	bots     []*Bot

	i18n *i18n.Bundle

	llmUpstreamHTTPClient *http.Client
	search                embeddings.EmbeddingSearch
	moduleManager         *erp_modules.ModuleManager
	moduleRegistry        *erp_modules.Registry
	intentAnalyzer        *erp_modules.LLMIntentAnalyzer
}

func resolveffmpegPath() string {
	_, standardPathErr := exec.LookPath("ffmpeg")
	if standardPathErr != nil {
		_, pluginPathErr := exec.LookPath(ffmpegPluginPath)
		if pluginPathErr != nil {
			return ""
		}
		return ffmpegPluginPath
	}

	return "ffmpeg"
}

func (p *Plugin) OnActivate() error {
	p.pluginAPI = pluginapi.NewClient(p.API, p.Driver)

	p.licenseChecker = enterprise.NewLicenseChecker(p.pluginAPI)

	p.metricsService = metrics.NewMetrics(metrics.InstanceInfo{
		InstallationID: os.Getenv("MM_CLOUD_INSTALLATION_ID"),
		PluginVersion:  manifest.Version,
	})
	p.metricsHandler = metrics.NewMetricsHandler(p.GetMetrics())

	p.i18n = i18nInit()

	p.llmUpstreamHTTPClient = httpservice.MakeHTTPServicePlugin(p.API).MakeClient(true)
	p.llmUpstreamHTTPClient.Timeout = time.Minute * 10 // LLM requests can be slow

	if err := p.MigrateServicesToBots(); err != nil {
		p.pluginAPI.Log.Error("failed to migrate services to bots", "error", err)
		// Don't fail on migration errors
	}

	if err := p.EnsureBots(); err != nil {
		p.pluginAPI.Log.Error("Failed to ensure bots", "error", err)
		// Don't fail on ensure bots errors as this leaves the plugin in an awkward state
		// where it can't be configured from the system console.
	}

	// Register slash commands
	if err := p.registerSlashCommands(); err != nil {
		p.pluginAPI.Log.Error("Failed to register slash commands", "error", err)
		// Continue even if command registration fails
	}

	if err := p.SetupDB(); err != nil {
		return err
	}

	var err error
	p.prompts, err = llm.NewPrompts(promptsFolder)
	if err != nil {
		return err
	}

	p.ffmpegPath = resolveffmpegPath()
	if p.ffmpegPath == "" {
		p.pluginAPI.Log.Error("ffmpeg not installed, transcriptions will be disabled.", "error", err)
	}

	p.streamingContexts = map[string]PostStreamContext{}

	// Initialize search if configured
	p.search, err = p.initSearch()
	if err != nil {
		// Only log the error but don't fail plugin activation
		p.pluginAPI.Log.Error("Failed to initialize search, search features will be disabled", "error", err)
	}

	// Initialize ERP modules
	if err := p.initializeERPModules(); err != nil {
		p.pluginAPI.Log.Error("Failed to initialize ERP modules", "error", err)
		// Continue activation even if ERP modules fail to initialize
	}

	return nil
}

// NewVectorStore creates a new vector store based on the provided configuration
func (p *Plugin) newVectorStore(config embeddings.UpstreamConfig, dimensions int) (embeddings.VectorStore, error) {
	switch config.Type { //nolint:gocritic
	case "pgvector":
		// Only available for PostgreSQL
		if !p.IsPostgreSQLDatabase() {
			return nil, fmt.Errorf("pgvector is only available for PostgreSQL databases, current database: %s", p.GetDatabaseType())
		}

		pgVectorConfig := postgres.PGVectorConfig{
			Dimensions: dimensions,
		}
		if err := json.Unmarshal(config.Parameters, &pgVectorConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal pgvector config: %w", err)
		}
		return postgres.NewPGVector(p.db, pgVectorConfig)
	}

	return nil, fmt.Errorf("unsupported vector store type: %s", config.Type)
}

// NewEmbeddingProvider creates a new embedding provider based on the provided configuration
func (p *Plugin) newEmbeddingProvider(config embeddings.UpstreamConfig) (embeddings.EmbeddingProvider, error) {
	switch config.Type {
	case "openai-compatible":
		compatibleConfig := openai.Config{}
		if err := json.Unmarshal(config.Parameters, &compatibleConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal OpenAI-compatible config: %w", err)
		}
		return openai.NewCompatibleEmbeddings(compatibleConfig, p.llmUpstreamHTTPClient), nil
	case "openai":
		var openaiConfig openai.Config
		if err := json.Unmarshal(config.Parameters, &openaiConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal OpenAI config: %w", err)
		}
		// Add the default OpenAI API URL
		if openaiConfig.APIURL == "" {
			openaiConfig.APIURL = "https://api.openai.com/v1"
		}
		// Set default embedding model and dimensions if not specified
		// if openaiConfig.EmbeddingModel == "" {
		// 	openaiConfig.EmbeddingModel = "text-embedding-3-small"
		// 	openaiConfig.EmbeddingDimentions = 1536
		// }
		return openai.NewCompatibleEmbeddings(openaiConfig, p.llmUpstreamHTTPClient), nil
	}

	return nil, fmt.Errorf("unsupported embedding provider type: %s", config.Type)
}

func (p *Plugin) initSearch() (embeddings.EmbeddingSearch, error) {
	cfg := p.getConfiguration()

	if cfg.EmbeddingSearchConfig.Type == "" {
		return nil, fmt.Errorf("search is disabled")
	}

	if !p.licenseChecker.IsBasicsLicensed() {
		return nil, fmt.Errorf("search is unavailable without a valid license")
	}

	// Check database compatibility for vector search
	if !p.IsPostgreSQLDatabase() {
		return nil, fmt.Errorf("embedding search is only available with PostgreSQL database, current database: %s", p.GetDatabaseType())
	}

	switch cfg.EmbeddingSearchConfig.Type {
	case "composite":
		// Validate dimensions
		if cfg.EmbeddingSearchConfig.Dimensions <= 0 {
			return nil, fmt.Errorf("invalid embedding dimensions: %d", cfg.EmbeddingSearchConfig.Dimensions)
		}

		p.pluginAPI.Log.Info("Initializing search with dimensions",
			"dimensions", cfg.EmbeddingSearchConfig.Dimensions)

		vector, err := p.newVectorStore(cfg.EmbeddingSearchConfig.VectorStore, cfg.EmbeddingSearchConfig.Dimensions)
		if err != nil {
			return nil, err
		}
		embeddor, err := p.newEmbeddingProvider(cfg.EmbeddingSearchConfig.EmbeddingProvider)
		if err != nil {
			return nil, err
		}

		// Validate that embedding provider dimensions match configuration
		if embeddor.Dimensions() != cfg.EmbeddingSearchConfig.Dimensions {
			p.pluginAPI.Log.Warn("Dimension mismatch detected",
				"config_dimensions", cfg.EmbeddingSearchConfig.Dimensions,
				"provider_dimensions", embeddor.Dimensions())
		}

		// Check if we have specific chunking options configured
		chunkingOpts := cfg.EmbeddingSearchConfig.ChunkingOptions
		if chunkingOpts.ChunkSize == 0 {
			chunkingOpts = embeddings.DefaultChunkingOptions()
		}

		return embeddings.NewCompositeSearch(vector, embeddor, chunkingOpts), nil
	}

	return nil, fmt.Errorf("unsupported search type: %s", cfg.EmbeddingSearchConfig.Type)
}

func (p *Plugin) getLLM(llmBotConfig llm.BotConfig) llm.LanguageModel {
	llmMetrics := p.metricsService.GetMetricsForAIService(llmBotConfig.Name)

	var result llm.LanguageModel
	switch llmBotConfig.Service.Type {
	case llm.ServiceTypeOpenAI:
		result = openai.New(llmBotConfig.Service, p.llmUpstreamHTTPClient, llmMetrics)
	case llm.ServiceTypeOpenAICompatible:
		result = openai.NewCompatible(llmBotConfig.Service, p.llmUpstreamHTTPClient, llmMetrics)
	case llm.ServiceTypeAzure:
		result = openai.NewAzure(llmBotConfig.Service, p.llmUpstreamHTTPClient, llmMetrics)
	case llm.ServiceTypeAnthropic:
		result = anthropic.New(llmBotConfig.Service, p.llmUpstreamHTTPClient, llmMetrics)
	}

	cfg := p.getConfiguration()
	if cfg.EnableLLMTrace {
		result = NewLanguageModelLogWrapper(p.pluginAPI.Log, result)
	}

	result = NewLLMTruncationWrapper(result)

	return result
}

func (p *Plugin) getTranscribe() Transcriber {
	cfg := p.getConfiguration()
	var botConfig llm.BotConfig
	for _, bot := range cfg.Bots {
		if bot.Name == cfg.TranscriptGenerator {
			botConfig = bot
			break
		}
	}
	llmMetrics := p.metricsService.GetMetricsForAIService(botConfig.Name)
	switch botConfig.Service.Type {
	case "openai":
		return openai.New(botConfig.Service, p.llmUpstreamHTTPClient, llmMetrics)
	case "openaicompatible":
		return openai.NewCompatible(botConfig.Service, p.llmUpstreamHTTPClient, llmMetrics)
	case "azure":
		return openai.NewAzure(botConfig.Service, p.llmUpstreamHTTPClient, llmMetrics)
	}
	return nil
}

// initializeERPModules initializes all ERP modules with enhanced intent analysis
func (p *Plugin) initializeERPModules() error {
	// Create registry
	p.moduleRegistry = erp_modules.NewRegistry()

	// Create enhanced intent analyzer with LLM provider
	p.intentAnalyzer = erp_modules.NewLLMIntentAnalyzer(
		func() llm.LanguageModel { return p.getLLM(p.getDefaultBot().cfg) },
		p.prompts,
		p.moduleRegistry,
	)

	// Create module manager with enhanced dependencies
	p.moduleManager = erp_modules.NewModuleManager(
		p.moduleRegistry,
		p.intentAnalyzer,
		func() llm.LanguageModel { return p.getLLM(p.getDefaultBot().cfg) },
		p.prompts,
	)

	// Initialize attendance module
	if err := p.initializeAttendanceModule(); err != nil {
		return fmt.Errorf("failed to initialize attendance module: %w", err)
	}

	return nil
}

// initializeAttendanceModule initializes the attendance module
func (p *Plugin) initializeAttendanceModule() error {
	config := p.getConfiguration()

	// Create attendance config
	attendanceConfig := attendance.AttendanceConfig{
		ERPDomain:      config.RollCall.ERPDomain,
		ERPAPIKey:      config.RollCall.ERPAPIKey,
		ERPAPISecret:   config.RollCall.ERPAPISecret,
		NotifyChannels: config.RollCall.NotifyChannels,
		Enabled:        config.RollCall.Enabled,
	}

	// Create i18n adapter
	i18nAdapter := &I18nAdapter{bundle: p.i18n}

	// Create plugin API adapter
	apiAdapter := &PluginAPIAdapter{plugin: p}

	// Get the default bot user ID
	defaultBot := p.getDefaultBot()
	var botUserID string
	if defaultBot != nil && defaultBot.mmBot != nil {
		botUserID = defaultBot.mmBot.UserId
	}

	// Create attendance module
	attendanceModule := attendance.NewAttendanceModule(
		attendanceConfig,
		p.llmUpstreamHTTPClient,
		i18nAdapter,
		p.prompts,
		func() llm.LanguageModel { return p.getLLM(p.getDefaultBot().cfg) },
		p.sendAttendanceNotification,
		apiAdapter,
		botUserID,
	)

	// Register the module
	return p.moduleRegistry.RegisterModule(attendanceModule)
}

// getDefaultBot returns the default bot configuration
func (p *Plugin) getDefaultBot() *Bot {
	p.botsLock.RLock()
	defer p.botsLock.RUnlock()

	if len(p.bots) > 0 {
		return p.bots[0]
	}
	return nil
}

// sendAttendanceNotification sends attendance notifications
func (p *Plugin) sendAttendanceNotification(userID, employeeName string, eventType attendance.RollCallEventType, eventTime string, reason string) error {
	// Get attendance module to send notification
	if module, exists := p.moduleRegistry.GetModule("attendance"); exists {
		if attendanceModule, ok := module.(*attendance.AttendanceModule); ok {
			return attendanceModule.SendNotification(userID, employeeName, eventType, eventTime, reason)
		}
	}
	return fmt.Errorf("attendance module not found")
}

// processERPRequest processes user requests that might be ERP-related with enhanced confidence handling
func (p *Plugin) processERPRequest(bot *Bot, user *model.User, channel *model.Channel, post *model.Post, llmContext *llm.Context) (*erp_modules.ModuleResponse, error) {
	if p.moduleManager == nil {
		p.API.LogError("Module manager not initialized")
		return nil, fmt.Errorf("module manager not initialized")
	}

	p.API.LogInfo("Processing ERP request",
		"user_id", user.Id,
		"username", user.Username,
		"message", post.Message,
		"channel_id", channel.Id)

	// Check if this is an ERP-related request by trying to process it
	response, err := p.moduleManager.ProcessUserRequest(
		context.Background(),
		post.Message,
		user,
		channel,
		post,
		llmContext,
	)

	// Handle different response types based on confidence and confirmation state
	if err != nil {
		// Log error but don't treat as ERP request failure
		p.API.LogError("Error processing ERP request", "error", err.Error())
		return nil, nil // Not an ERP request
	}

	if response != nil {
		p.API.LogInfo("ERP response received",
			"success", response.Success,
			"message", response.Message,
			"action_taken", response.ActionTaken)

		// Log response data for debugging
		if response.Data != nil {
			for key, value := range response.Data {
				p.API.LogInfo("ERP response data", "key", key, "value", fmt.Sprintf("%v", value))
			}
		}

		// Check if this is a confirmation request
		if data, ok := response.Data["awaiting_confirmation"]; ok && data.(bool) {
			p.API.LogInfo("This is a confirmation request")
			return response, nil
		}

		// Check if this is a general response (low confidence)
		if actionTaken, ok := response.Data["type"]; ok && actionTaken == "general_knowledge" {
			p.API.LogInfo("This was handled as general knowledge, not an ERP request")
			return nil, nil
		}

		// This is a successful ERP request
		if response.Success {
			p.API.LogInfo("ERP request processed successfully")
			return response, nil
		}

		// ERP request failed
		if response.Error == "Low confidence in intent analysis" {
			p.API.LogInfo("Low confidence in intent analysis, treating as non-ERP request")
			return nil, nil // Not an ERP request
		}
	} else {
		p.API.LogInfo("No response from ERP processing")
	}

	return response, err
}

// Helper function to check if a user has pending confirmations
func (p *Plugin) userHasPendingConfirmation(userID string) bool {
	if p.intentAnalyzer == nil {
		return false
	}
	return p.intentAnalyzer.HasPendingConfirmation(userID)
}

// Helper function to clear pending confirmations (useful for cleanup)
func (p *Plugin) clearUserPendingConfirmation(userID string) {
	if p.intentAnalyzer != nil {
		p.intentAnalyzer.ClearPendingConfirmation(userID)
	}
}

// I18nAdapter adapts the i18n bundle to the interface needed by modules
type I18nAdapter struct {
	bundle *i18n.Bundle
}

func (a *I18nAdapter) Localize(messageID, defaultMessage, locale string, params ...interface{}) string {
	localizer := i18n.NewLocalizer(a.bundle, locale)
	if len(params) > 0 {
		return fmt.Sprintf(localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    messageID,
				Other: defaultMessage,
			},
		}), params...)
	}
	return localizer.MustLocalize(&i18n.LocalizeConfig{
		DefaultMessage: &i18n.Message{
			ID:    messageID,
			Other: defaultMessage,
		},
	})
}

// PluginAPIAdapter adapts the plugin API to the interface needed by modules
type PluginAPIAdapter struct {
	plugin *Plugin
}

func (a *PluginAPIAdapter) LogDebug(message string, keyValuePairs ...interface{}) {
	a.plugin.API.LogDebug(message, keyValuePairs...)
}

func (a *PluginAPIAdapter) LogError(message string, keyValuePairs ...interface{}) {
	a.plugin.API.LogError(message, keyValuePairs...)
}

func (a *PluginAPIAdapter) LogInfo(message string, keyValuePairs ...interface{}) {
	a.plugin.API.LogInfo(message, keyValuePairs...)
}

func (a *PluginAPIAdapter) LogWarn(message string, keyValuePairs ...interface{}) {
	a.plugin.API.LogWarn(message, keyValuePairs...)
}

func (a *PluginAPIAdapter) GetUser(userID string) (*model.User, error) {
	return a.plugin.pluginAPI.User.Get(userID)
}

func (a *PluginAPIAdapter) CreatePost(post *model.Post) error {
	return a.plugin.pluginAPI.Post.CreatePost(post)
}

func (a *PluginAPIAdapter) GetConfig() *model.Config {
	return a.plugin.API.GetConfig()
}

func (a *PluginAPIAdapter) BotDMNonResponse(botUserID, userID string, post *model.Post) error {
	return a.plugin.botDMNonResponse(botUserID, userID, post)
}

func (a *PluginAPIAdapter) GetChannel(channelID string) (*model.Channel, error) {
	return a.plugin.pluginAPI.Channel.Get(channelID)
}

func (a *PluginAPIAdapter) GetChannelMember(channelID, userID string) (*model.ChannelMember, error) {
	return a.plugin.pluginAPI.Channel.GetMember(channelID, userID)
}

func (a *PluginAPIAdapter) AddChannelMember(channelID, userID string) (*model.ChannelMember, error) {
	return a.plugin.pluginAPI.Channel.AddMember(channelID, userID)
}
