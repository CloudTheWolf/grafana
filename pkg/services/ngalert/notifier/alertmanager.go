package notifier

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	alertingNotify "github.com/grafana/alerting/notify"
	"github.com/grafana/alerting/receivers"
	alertingTemplates "github.com/grafana/alerting/templates"
	"github.com/prometheus/alertmanager/config"

	amv2 "github.com/prometheus/alertmanager/api/v2/models"

	"github.com/grafana/grafana/pkg/infra/kvstore"
	"github.com/grafana/grafana/pkg/infra/log"
	apimodels "github.com/grafana/grafana/pkg/services/ngalert/api/tooling/definitions"
	"github.com/grafana/grafana/pkg/services/ngalert/metrics"
	ngmodels "github.com/grafana/grafana/pkg/services/ngalert/models"
	"github.com/grafana/grafana/pkg/services/ngalert/store"
	"github.com/grafana/grafana/pkg/services/notifications"
	"github.com/grafana/grafana/pkg/setting"
)

const (
	NotificationLogFilename = "notifications"
	SilencesFilename        = "silences"

	workingDir = "alerting"
	// maintenanceNotificationAndSilences how often should we flush and garbage collect notifications
	notificationLogMaintenanceInterval = 15 * time.Minute
)

// How long should we keep silences and notification entries on-disk after they've served their purpose.
var retentionNotificationsAndSilences = 5 * 24 * time.Hour
var silenceMaintenanceInterval = 15 * time.Minute

type AlertingStore interface {
	store.AlertingStore
	store.ImageStore
}

type alertmanager struct {
	Base   *alertingNotify.GrafanaAlertmanager
	logger log.Logger

	ConfigMetrics       *metrics.AlertmanagerConfigMetrics
	Settings            *setting.Cfg
	Store               AlertingStore
	fileStore           *FileStore
	NotificationService notifications.Service

	decryptFn alertingNotify.GetDecryptedValueFn
	orgID     int64
}

// maintenanceOptions represent the options for components that need maintenance on a frequency within the Alertmanager.
// It implements the alerting.MaintenanceOptions interface.
type maintenanceOptions struct {
	filepath             string
	retention            time.Duration
	maintenanceFrequency time.Duration
	maintenanceFunc      func(alertingNotify.State) (int64, error)
}

func (m maintenanceOptions) Filepath() string {
	return m.filepath
}

func (m maintenanceOptions) Retention() time.Duration {
	return m.retention
}

func (m maintenanceOptions) MaintenanceFrequency() time.Duration {
	return m.maintenanceFrequency
}

func (m maintenanceOptions) MaintenanceFunc(state alertingNotify.State) (int64, error) {
	return m.maintenanceFunc(state)
}

func NewAlertmanager(ctx context.Context, orgID int64, cfg *setting.Cfg, store AlertingStore, kvStore kvstore.KVStore,
	peer alertingNotify.ClusterPeer, decryptFn alertingNotify.GetDecryptedValueFn, ns notifications.Service,
	m *metrics.Alertmanager) (*alertmanager, error) {
	workingPath := filepath.Join(cfg.DataPath, workingDir, strconv.Itoa(int(orgID)))
	fileStore := NewFileStore(orgID, kvStore, workingPath)

	nflogFilepath, err := fileStore.FilepathFor(ctx, NotificationLogFilename)
	if err != nil {
		return nil, err
	}
	silencesFilepath, err := fileStore.FilepathFor(ctx, SilencesFilename)
	if err != nil {
		return nil, err
	}

	silencesOptions := maintenanceOptions{
		filepath:             silencesFilepath,
		retention:            retentionNotificationsAndSilences,
		maintenanceFrequency: silenceMaintenanceInterval,
		maintenanceFunc: func(state alertingNotify.State) (int64, error) {
			// Detached context here is to make sure that when the service is shut down the persist operation is executed.
			return fileStore.Persist(context.Background(), SilencesFilename, state)
		},
	}

	nflogOptions := maintenanceOptions{
		filepath:             nflogFilepath,
		retention:            retentionNotificationsAndSilences,
		maintenanceFrequency: notificationLogMaintenanceInterval,
		maintenanceFunc: func(state alertingNotify.State) (int64, error) {
			// Detached context here is to make sure that when the service is shut down the persist operation is executed.
			return fileStore.Persist(context.Background(), NotificationLogFilename, state)
		},
	}

	amcfg := &alertingNotify.GrafanaAlertmanagerConfig{
		WorkingDirectory:   filepath.Join(cfg.DataPath, workingDir, strconv.Itoa(int(orgID))),
		ExternalURL:        cfg.AppURL,
		AlertStoreCallback: nil,
		PeerTimeout:        cfg.UnifiedAlerting.HAPeerTimeout,
		Silences:           silencesOptions,
		Nflog:              nflogOptions,
	}

	l := log.New("ngalert.notifier.alertmanager", "org", orgID)
	gam, err := alertingNotify.NewGrafanaAlertmanager("orgID", orgID, amcfg, peer, l, alertingNotify.NewGrafanaAlertmanagerMetrics(m.Registerer))
	if err != nil {
		return nil, err
	}

	am := &alertmanager{
		Base:                gam,
		ConfigMetrics:       m.AlertmanagerConfigMetrics,
		Settings:            cfg,
		Store:               store,
		NotificationService: ns,
		orgID:               orgID,
		decryptFn:           decryptFn,
		fileStore:           fileStore,
		logger:              l,
	}

	return am, nil
}

func (am *alertmanager) Ready() bool {
	// We consider AM as ready only when the config has been
	// applied at least once successfully. Until then, some objects
	// can still be nil.
	return am.Base.Ready()
}

func (am *alertmanager) StopAndWait() {
	am.Base.StopAndWait()
}

// SaveAndApplyDefaultConfig saves the default configuration to the database and applies it to the Alertmanager.
// It rolls back the save if we fail to apply the configuration.
func (am *alertmanager) SaveAndApplyDefaultConfig(ctx context.Context) error {
	var outerErr error
	am.Base.WithLock(func() {
		cmd := &ngmodels.SaveAlertmanagerConfigurationCmd{
			AlertmanagerConfiguration: am.Settings.UnifiedAlerting.DefaultConfiguration,
			Default:                   true,
			ConfigurationVersion:      fmt.Sprintf("v%d", ngmodels.AlertConfigurationVersion),
			OrgID:                     am.orgID,
			LastApplied:               time.Now().UTC().Unix(),
		}

		cfg, err := Load([]byte(am.Settings.UnifiedAlerting.DefaultConfiguration))
		if err != nil {
			outerErr = err
			return
		}

		err = am.Store.SaveAlertmanagerConfigurationWithCallback(ctx, cmd, func() error {
			_, err := am.applyConfig(cfg, []byte(am.Settings.UnifiedAlerting.DefaultConfiguration))
			return err
		})
		if err != nil {
			outerErr = nil
			return
		}
	})

	return outerErr
}

// SaveAndApplyConfig saves the configuration the database and applies the configuration to the Alertmanager.
// It rollbacks the save if we fail to apply the configuration.
func (am *alertmanager) SaveAndApplyConfig(ctx context.Context, cfg *apimodels.PostableUserConfig) error {
	rawConfig, err := json.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("failed to serialize to the Alertmanager configuration: %w", err)
	}

	var outerErr error
	am.Base.WithLock(func() {
		cmd := &ngmodels.SaveAlertmanagerConfigurationCmd{
			AlertmanagerConfiguration: string(rawConfig),
			ConfigurationVersion:      fmt.Sprintf("v%d", ngmodels.AlertConfigurationVersion),
			OrgID:                     am.orgID,
			LastApplied:               time.Now().UTC().Unix(),
		}

		err = am.Store.SaveAlertmanagerConfigurationWithCallback(ctx, cmd, func() error {
			_, err := am.applyConfig(cfg, rawConfig)
			return err
		})
		if err != nil {
			outerErr = err
			return
		}
	})

	return outerErr
}

// ApplyConfig applies the configuration to the Alertmanager.
func (am *alertmanager) ApplyConfig(ctx context.Context, dbCfg *ngmodels.AlertConfiguration) error {
	var err error
	cfg, err := Load([]byte(dbCfg.AlertmanagerConfiguration))
	if err != nil {
		return fmt.Errorf("failed to parse Alertmanager config: %w", err)
	}

	var outerErr error
	am.Base.WithLock(func() {
		if err := am.applyAndMarkConfig(ctx, dbCfg.ConfigurationHash, cfg, nil); err != nil {
			outerErr = fmt.Errorf("unable to apply configuration: %w", err)
			return
		}
	})

	return outerErr
}

type AggregateMatchersUsage struct {
	Matchers       int
	MatchRE        int
	Match          int
	ObjectMatchers int
}

func (am *alertmanager) updateConfigMetrics(cfg *apimodels.PostableUserConfig) {
	var amu AggregateMatchersUsage
	am.aggregateRouteMatchers(cfg.AlertmanagerConfig.Route, &amu)
	am.aggregateInhibitMatchers(cfg.AlertmanagerConfig.InhibitRules, &amu)
	am.ConfigMetrics.Matchers.Set(float64(amu.Matchers))
	am.ConfigMetrics.MatchRE.Set(float64(amu.MatchRE))
	am.ConfigMetrics.Match.Set(float64(amu.Match))
	am.ConfigMetrics.ObjectMatchers.Set(float64(amu.ObjectMatchers))

	am.ConfigMetrics.ConfigHash.
		WithLabelValues(strconv.FormatInt(am.orgID, 10)).
		Set(hashAsMetricValue(am.Base.ConfigHash()))
}

func (am *alertmanager) aggregateRouteMatchers(r *apimodels.Route, amu *AggregateMatchersUsage) {
	amu.Matchers += len(r.Matchers)
	amu.MatchRE += len(r.MatchRE)
	amu.Match += len(r.Match)
	amu.ObjectMatchers += len(r.ObjectMatchers)
	for _, next := range r.Routes {
		am.aggregateRouteMatchers(next, amu)
	}
}

func (am *alertmanager) aggregateInhibitMatchers(rules []config.InhibitRule, amu *AggregateMatchersUsage) {
	for _, r := range rules {
		amu.Matchers += len(r.SourceMatchers)
		amu.Matchers += len(r.TargetMatchers)
		amu.MatchRE += len(r.SourceMatchRE)
		amu.MatchRE += len(r.TargetMatchRE)
		amu.Match += len(r.SourceMatch)
		amu.Match += len(r.TargetMatch)
	}
}

// applyConfig applies a new configuration by re-initializing all components using the configuration provided.
// It returns a boolean indicating whether the user config was changed and an error.
// It is not safe to call concurrently.
func (am *alertmanager) applyConfig(cfg *apimodels.PostableUserConfig, rawConfig []byte) (bool, error) {
	// First, let's make sure this config is not already loaded
	var amConfigChanged bool
	if rawConfig == nil {
		enc, err := json.Marshal(cfg.AlertmanagerConfig)
		if err != nil {
			// In theory, this should never happen.
			return false, err
		}
		rawConfig = enc
	}

	if am.Base.ConfigHash() != md5.Sum(rawConfig) {
		amConfigChanged = true
	}

	if cfg.TemplateFiles == nil {
		cfg.TemplateFiles = map[string]string{}
	}
	cfg.TemplateFiles["__default__.tmpl"] = alertingTemplates.DefaultTemplateString

	// next, we need to make sure we persist the templates to disk.
	paths, templatesChanged, err := PersistTemplates(am.logger, cfg, am.Base.WorkingDirectory())
	if err != nil {
		return false, err
	}
	cfg.AlertmanagerConfig.Templates = paths

	// If neither the configuration nor templates have changed, we've got nothing to do.
	if !amConfigChanged && !templatesChanged {
		am.logger.Debug("Neither config nor template have changed, skipping configuration sync.")
		return false, nil
	}

	err = am.Base.ApplyConfig(AlertingConfiguration{
		rawAlertmanagerConfig:    rawConfig,
		alertmanagerConfig:       cfg.AlertmanagerConfig,
		receivers:                PostableApiAlertingConfigToApiReceivers(cfg.AlertmanagerConfig),
		receiverIntegrationsFunc: am.buildReceiverIntegrations,
	})
	if err != nil {
		return false, err
	}

	am.updateConfigMetrics(cfg)
	return true, nil
}

// applyAndMarkConfig applies a configuration and marks it as applied if no errors occur.
func (am *alertmanager) applyAndMarkConfig(ctx context.Context, hash string, cfg *apimodels.PostableUserConfig, rawConfig []byte) error {
	configChanged, err := am.applyConfig(cfg, rawConfig)
	if err != nil {
		return err
	}

	if configChanged {
		markConfigCmd := ngmodels.MarkConfigurationAsAppliedCmd{
			OrgID:             am.orgID,
			ConfigurationHash: hash,
		}
		return am.Store.MarkConfigurationAsApplied(ctx, &markConfigCmd)
	}

	return nil
}

func (am *alertmanager) AppURL() string {
	return am.Settings.AppURL
}

// buildReceiverIntegrations builds a list of integration notifiers off of a receiver config.
func (am *alertmanager) buildReceiverIntegrations(receiver *alertingNotify.APIReceiver, tmpl *alertingTemplates.Template) ([]*alertingNotify.Integration, error) {
	receiverCfg, err := alertingNotify.BuildReceiverConfiguration(context.Background(), receiver, am.decryptFn)
	if err != nil {
		return nil, err
	}
	s := &sender{am.NotificationService}
	img := newImageProvider(am.Store, log.New("ngalert.notifier.image-provider"))
	integrations, err := alertingNotify.BuildReceiverIntegrations(
		receiverCfg,
		tmpl,
		img,
		LoggerFactory,
		func(n receivers.Metadata) (receivers.WebhookSender, error) {
			return s, nil
		},
		func(n receivers.Metadata) (receivers.EmailSender, error) {
			return s, nil
		},
		am.orgID,
		setting.BuildVersion,
	)
	if err != nil {
		return nil, err
	}
	return integrations, nil
}

// PutAlerts receives the alerts and then sends them through the corresponding route based on whenever the alert has a receiver embedded or not
func (am *alertmanager) PutAlerts(_ context.Context, postableAlerts apimodels.PostableAlerts) error {
	alerts := make(alertingNotify.PostableAlerts, 0, len(postableAlerts.PostableAlerts))
	for _, pa := range postableAlerts.PostableAlerts {
		alerts = append(alerts, &alertingNotify.PostableAlert{
			Annotations: pa.Annotations,
			EndsAt:      pa.EndsAt,
			StartsAt:    pa.StartsAt,
			Alert:       pa.Alert,
		})
	}

	return am.Base.PutAlerts(alerts)
}

// CleanUp removes the directory containing the alertmanager files from disk.
func (am *alertmanager) CleanUp() {
	am.fileStore.CleanUp()
}

// AlertValidationError is the error capturing the validation errors
// faced on the alerts.
type AlertValidationError struct {
	Alerts []amv2.PostableAlert
	Errors []error // Errors[i] refers to Alerts[i].
}

func (e AlertValidationError) Error() string {
	errMsg := ""
	if len(e.Errors) != 0 {
		errMsg = e.Errors[0].Error()
		for _, e := range e.Errors[1:] {
			errMsg += ";" + e.Error()
		}
	}
	return errMsg
}

type nilLimits struct{}

func (n nilLimits) MaxNumberOfAggregationGroups() int { return 0 }

// This function is taken from upstream, modified to take a [16]byte instead of a []byte.
// https://github.com/prometheus/alertmanager/blob/30fa9cd44bc91c0d6adcc9985609bb08a09a127b/config/coordinator.go#L149-L156
func hashAsMetricValue(data [16]byte) float64 {
	// We only want 48 bits as a float64 only has a 53 bit mantissa.
	smallSum := data[0:6]
	bytes := make([]byte, 8)
	copy(bytes, smallSum)
	return float64(binary.LittleEndian.Uint64(bytes))
}
