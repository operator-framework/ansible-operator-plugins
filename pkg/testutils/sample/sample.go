package sample

import (
	"github.com/operator-framework/ansible-operator-plugins/pkg/testutils/command"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Sample represents a sample project that can be created and used for testing
type Sample interface {
	// CommandContext returns the CommandContext that the Sample is using
	CommandContext() command.CommandContext
	// Name returns the name of the Sample
	Name() string
	// GVKs return an array of GVKs that are used when generating the apis and webhooks for the Sample
	GVKs() []schema.GroupVersionKind
	// Domain returs the domain of the sample
	Domain() string
	// Dir returns the directory the sample is created in
	Dir() string
	// Binary returns the binary that is used when creating a sample
	Binary() string
	// GenerateInit scaffolds using the `init` subcommand
	GenerateInit() error
	// GenerateApi scaffolds using the `create api` subcommand
	GenerateApi() error
	// GenerateWebhook scaffolds using the `create webhook` subcommand
	GenerateWebhook() error
}
