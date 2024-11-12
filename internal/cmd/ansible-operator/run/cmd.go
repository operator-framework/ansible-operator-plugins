// Copyright 2020 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package run

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	zapf "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/operator-framework/ansible-operator-plugins/internal/ansible/apiserver"
	"github.com/operator-framework/ansible-operator-plugins/internal/ansible/controller"
	"github.com/operator-framework/ansible-operator-plugins/internal/ansible/events"
	"github.com/operator-framework/ansible-operator-plugins/internal/ansible/flags"
	"github.com/operator-framework/ansible-operator-plugins/internal/ansible/metrics"
	"github.com/operator-framework/ansible-operator-plugins/internal/ansible/proxy"
	"github.com/operator-framework/ansible-operator-plugins/internal/ansible/proxy/controllermap"
	"github.com/operator-framework/ansible-operator-plugins/internal/ansible/runner"
	"github.com/operator-framework/ansible-operator-plugins/internal/ansible/watches"
	"github.com/operator-framework/ansible-operator-plugins/internal/util/k8sutil"
	sdkVersion "github.com/operator-framework/ansible-operator-plugins/internal/version"
)

var log = logf.Log.WithName("cmd")

func printVersion() {
	version := sdkVersion.GitVersion
	if version == "unknown" {
		version = sdkVersion.Version
	}
	log.Info("Version",
		"Go Version", runtime.Version(),
		"GOOS", runtime.GOOS,
		"GOARCH", runtime.GOARCH,
		"ansible-operator", version,
		"commit", sdkVersion.GitCommit)
}

func NewCmd() *cobra.Command {
	f := &flags.Flags{}
	zapfs := flag.NewFlagSet("zap", flag.ExitOnError)
	opts := &zapf.Options{}
	opts.BindFlags(zapfs)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the operator",
		Args: func(cmd *cobra.Command, args []string) error {
			if cmd.Flag("metrics-require-rbac").Value.String() == "true" && cmd.Flag("metrics-secure").Value.String() == "false" {
				return errors.New("--metrics-secure flag is required when --metrics-require-rbac is present")
			}
			return nil
		},
		Run: func(cmd *cobra.Command, _ []string) {
			logf.SetLogger(zapf.New(zapf.UseFlagOptions(opts)))
			run(cmd, f)
		},
	}

	f.AddTo(cmd.Flags())
	cmd.Flags().AddGoFlagSet(zapfs)
	return cmd
}

func run(cmd *cobra.Command, f *flags.Flags) {
	printVersion()
	metrics.RegisterBuildInfo(crmetrics.Registry)

	// Load config options from the config at f.ManagerConfigPath.
	// These options will not override those set by flags.
	var (
		options manager.Options
		err     error
	)
	exitIfUnsupported(options)

	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "Failed to get config.")
		os.Exit(1)
	}

	// TODO(2.0.0): remove
	// Deprecated: OPERATOR_NAME environment variable is an artifact of the
	// legacy operator-sdk project scaffolding. Flag `--leader-election-id`
	// should be used instead.
	if operatorName, found := os.LookupEnv("OPERATOR_NAME"); found {
		log.Info("Environment variable OPERATOR_NAME has been deprecated, use --leader-election-id instead.")
		if cmd.Flags().Changed("leader-election-id") {
			log.Info("Ignoring OPERATOR_NAME environment variable since --leader-election-id is set")
		} else if options.LeaderElectionID == "" {
			// Only set leader election ID using OPERATOR_NAME if unset everywhere else,
			// since this env var is deprecated.
			options.LeaderElectionID = operatorName
		}
	}

	//TODO(2.0.0): remove the following checks. they are required just because of the flags deprecation
	if cmd.Flags().Changed("leader-elect") && cmd.Flags().Changed("enable-leader-election") {
		log.Error(errors.New("only one of --leader-elect and --enable-leader-election may be set"), "invalid flags usage")
		os.Exit(1)
	}

	if cmd.Flags().Changed("metrics-addr") && cmd.Flags().Changed("metrics-bind-address") {
		log.Error(errors.New("only one of --metrics-addr and --metrics-bind-address may be set"), "invalid flags usage")
		os.Exit(1)
	}

	// Set default manager options
	// TODO: probably should expose the host & port as an environment variables
	options = f.ToManagerOptions(options)
	if options.NewClient == nil {
		options.NewClient = client.New
	}

	configureWatchNamespaces(&options, log)

	err = setAnsibleEnvVars(f)
	if err != nil {
		log.Error(err, "Failed to set environment variable.")
		os.Exit(1)
	}

	// Create a new manager to provide shared dependencies and start components
	mgr, err := manager.New(cfg, options)
	if err != nil {
		log.Error(err, "Failed to create a new manager.")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		log.Error(err, "Unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		log.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}

	cMap := controllermap.NewControllerMap()
	watches, err := watches.Load(f.WatchesFile, f.MaxConcurrentReconciles, f.AnsibleVerbosity)
	if err != nil {
		log.Error(err, "Failed to load watches.")
		os.Exit(1)
	}
	for _, w := range watches {
		reconcilePeriod := f.ReconcilePeriod
		if w.ReconcilePeriod.Duration != time.Duration(0) {
			// if a duration other than default was passed in through watches,
			// it will take precedence over the command-line flag
			reconcilePeriod = w.ReconcilePeriod.Duration
		}

		runner, err := runner.New(w, f.AnsibleArgs)
		if err != nil {
			log.Error(err, "Failed to create runner")
			os.Exit(1)
		}

		ctr := controller.Add(mgr, controller.Options{
			GVK:                     w.GroupVersionKind,
			Runner:                  runner,
			ManageStatus:            w.ManageStatus,
			AnsibleDebugLogs:        getAnsibleDebugLog(),
			MaxConcurrentReconciles: w.MaxConcurrentReconciles,
			ReconcilePeriod:         reconcilePeriod,
			Selector:                w.Selector,
			LoggingLevel:            getAnsibleEventsToLog(f),
			WatchAnnotationsChanges: w.WatchAnnotationsChanges,
		})
		if ctr == nil {
			log.Error(fmt.Errorf("failed to add controller for GVK %v", w.GroupVersionKind.String()), "")
			os.Exit(1)
		}

		cMap.Store(w.GroupVersionKind, &controllermap.Contents{Controller: *ctr, //nolint:staticcheck
			WatchDependentResources:     w.WatchDependentResources,
			WatchClusterScopedResources: w.WatchClusterScopedResources,
			OwnerWatchMap:               controllermap.NewWatchMap(),
			AnnotationWatchMap:          controllermap.NewWatchMap(),
		}, w.Blacklist)
	}

	// TODO(2.0.0): remove
	err = mgr.AddHealthzCheck("ping", healthz.Ping)
	if err != nil {
		log.Error(err, "Failed to add Healthz check.")
	}

	done := make(chan error)

	// start the proxy
	err = proxy.Run(done, proxy.Options{
		Address:           "localhost",
		Port:              f.ProxyPort,
		KubeConfig:        mgr.GetConfig(),
		Scheme:            mgr.GetScheme(),
		Cache:             mgr.GetCache(),
		RESTMapper:        mgr.GetRESTMapper(),
		ControllerMap:     cMap,
		OwnerInjection:    f.InjectOwnerRef,
		WatchedNamespaces: options.Cache.DefaultNamespaces,
	})
	if err != nil {
		log.Error(err, "Error starting proxy.")
		os.Exit(1)
	}
	// start the ansible-operator api server
	go func() {
		err = apiserver.Run(apiserver.Options{
			Address: "localhost",
			Port:    5050,
		})
		done <- err
	}()

	// start the operator
	go func() {
		done <- mgr.Start(signals.SetupSignalHandler())
	}()

	// wait for either to finish
	err = <-done
	if err != nil {
		log.Error(err, "Proxy or operator exited with error.")
		os.Exit(1)
	}
	log.Info("Exiting.")
}

// exitIfUnsupported prints an error containing unsupported field names and exits
// if any of those fields are not their default values.
func exitIfUnsupported(options manager.Options) {
	var keys []string

	if options.WebhookServer != nil {
		// The below options are webhook-specific, which is not supported by ansible.
		// Adding logs only for the previously supported values through manager.
		if options.WebhookServer.(*webhook.DefaultServer).Options.CertDir != "" {
			keys = append(keys, "certDir")
		}
		if options.WebhookServer.(*webhook.DefaultServer).Options.Host != "" {
			keys = append(keys, "host")
		}
		if options.WebhookServer.(*webhook.DefaultServer).Options.Port != 0 {
			keys = append(keys, "port")
		}
		log.Error(fmt.Errorf(`options for setting webhook server configuration is not supported. 
		%s set in manager options`, strings.Join(keys, ", ")), "unsupported fields")
		os.Exit(1)
	}
}

// getAnsibleDebugLog return the value from the ANSIBLE_DEBUG_LOGS it order to
// print the full Ansible logs
func getAnsibleDebugLog() bool {
	const envVar = "ANSIBLE_DEBUG_LOGS"
	val := false
	if envVal, ok := os.LookupEnv(envVar); ok {
		if i, err := strconv.ParseBool(envVal); err != nil {
			log.Info("Could not parse environment variable as an boolean; using default value",
				"envVar", envVar, "default", val)
		} else {
			val = i
		}
	} else if !ok {
		log.Info("Environment variable not set; using default value", "envVar", envVar,
			envVar, val)
	}
	return val
}

// getAnsibleEventsToLog return the integer value of the log level set in the flag
func getAnsibleEventsToLog(f *flags.Flags) events.LogLevel {
	if strings.ToLower(f.AnsibleLogEvents) == "everything" {
		return events.Everything
	} else if strings.ToLower(f.AnsibleLogEvents) == "nothing" {
		return events.Nothing
	} else {
		if strings.ToLower(f.AnsibleLogEvents) != "tasks" && f.AnsibleLogEvents != "" {
			log.Error(fmt.Errorf("--ansible-log-events flag value '%s' not recognized. Must be one of: Tasks, Everything, Nothing", f.AnsibleLogEvents), "unrecognized log level")
		}
		return events.Tasks // Tasks is the default
	}
}

// setAnsibleEnvVars will set environment variables based on CLI flags
func setAnsibleEnvVars(f *flags.Flags) error {
	if len(f.AnsibleRolesPath) > 0 {
		if err := os.Setenv(flags.AnsibleRolesPathEnvVar, f.AnsibleRolesPath); err != nil {
			return fmt.Errorf("failed to set environment variable %s: %v", flags.AnsibleRolesPathEnvVar, err)
		}
		log.Info("Set the environment variable", "envVar", flags.AnsibleRolesPathEnvVar,
			"value", f.AnsibleRolesPath)
	}

	if len(f.AnsibleCollectionsPath) > 0 {
		if err := os.Setenv(flags.AnsibleCollectionsPathEnvVar, f.AnsibleCollectionsPath); err != nil {
			return fmt.Errorf("failed to set environment variable %s: %v", flags.AnsibleCollectionsPathEnvVar, err)
		}
		log.Info("Set the environment variable", "envVar", flags.AnsibleCollectionsPathEnvVar,
			"value", f.AnsibleCollectionsPath)
	}
	return nil
}

func configureWatchNamespaces(options *manager.Options, log logr.Logger) {
	namespaces := splitNamespaces(os.Getenv(k8sutil.WatchNamespaceEnvVar))

	namespaceConfigs := make(map[string]cache.Config)
	if len(namespaces) != 0 {
		log.Info("Watching namespaces", "namespaces", namespaces)
		for _, namespace := range namespaces {
			namespaceConfigs[namespace] = cache.Config{}
		}
	} else {
		log.Info("Watching all namespaces")
		namespaceConfigs[metav1.NamespaceAll] = cache.Config{}
	}

	options.Cache.DefaultNamespaces = namespaceConfigs
}

func splitNamespaces(namespaces string) []string {
	list := strings.Split(namespaces, ",")
	var out []string
	for _, ns := range list {
		trimmed := strings.TrimSpace(ns)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
