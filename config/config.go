package config

import (
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/hesusruiz/isbetmf/internal/errl"
	"github.com/hesusruiz/isbetmf/types"
)

// Indicates the environment (SBX, DEV2, PRO, LCL) where the server is running.
// It is used to determine the default configuration profile if the user does not specify anything else.
type Environment string

const (
	DOME_PRO Environment = "domepro"
	DOME_PRE Environment = "domepre"
	DOME_DEV Environment = "domedev"
	LOCAL    Environment = "local"
	ISBE_PRE Environment = "isbepre"
	ISBE_DEV Environment = "isbedev"
	ISBE_PRO Environment = "isbepro"
)

const DefaultClonePeriod = 10 * time.Minute

type Config struct {

	// The environment for the default configuration profile
	Environment Environment

	// Information about the organization operating this server
	ServerOperatorOrganizationIdentifier string
	ServerOperatorDid                    string
	ServerOperatorName                   string
	ServerOperatorCountry                string
	ServerEmailAddress                   string

	// VerifierServer is the URL of the verifier server, which is used to verify the access tokens.
	VerifierServer string

	// Dbname is the name of the database file where the local TMForum data is stored
	// It is used to store the data in a local SQLite database, the best SQL database for this purpose.
	Dbname string

	// The power required by a caller to be considered LEAR
	LEARPower types.OnePower

	// The powers required by a caller to be able to create, update and delete a product
	ProductCreatePower types.OnePower
	ProductUpdatePower types.OnePower
	ProductDeletePower types.OnePower

	// PolicyFileName is the name of the file where the user-defined policies are stored.
	// It can specify a local file or a remote URL.
	PolicyFileName string

	// Debug mode, more logs and less caching
	Debug bool

	// The admin token used to authenticate the superadmin
	// The admin token does not have to be based on a LEARCredential.
	// This is a special token defined in the configuration and has superadmin powers.
	AdminToken string

	// ClonePeriod is the period in which the reporting tool will clone the TMForum objects from the DOME instance,
	// to keep the local cache up to date.
	ClonePeriod time.Duration

	// Hour and minute of the day when the server will automatically restart (each day). Hour=-1 disables restart.
	RestartHour, RestartMinute int

	// ProxyEnabled enables the TMF caching proxy functionality.
	ProxyEnabled bool

	// The domain of the remote TMForum API server when we act as proxy
	RemoteTMFServer string

	// Enable synchronization with the remote server in background
	BackgroudSync bool

	// The special features of the environment
	Features Features
}

// Features defines a set of feature flags which may depend on the environment at a given time
type Features struct {
	// Only the server operator admin can launch an offering.
	OfferingLaunchOnlyByAdmin bool

	// GenerateIDOnCreate forces the server to generate an ID for the object on POST.
	GenerateIDOnCreate bool

	// AllowIDInBody allows the client to specify the ID of the object on POST.
	AllowIDInBody bool

	// VerifyJWTSignature verifies the signature of the JWT.
	VerifyJWTSignature bool
}

// LoadConfig initializes and returns a Config struct based on the provided parameters.
// It sets up logging, selects the appropriate environment, and applies configuration options.
//
// Parameters:
//   - envir:        The environment to use ("pro", "dev2", "sbx", "lcl").
//   - debug:        Enables debug logging if true.
//
// Returns:
//   - *Config: The initialized configuration struct.
//   - error:   An error if configuration or logger setup fails.
func LoadConfig(
	envir string,
	debug bool,
) (*Config, error) {
	var conf *Config

	// Normalize to lowercase for comparisons
	envir = strings.ToLower(envir)

	// The environment has precedence over the parameter
	if en := os.Getenv("ISBETMF_RUN_ENVIRONMENT"); en != "" {
		envir = strings.ToLower(en)
	}
	environment := Environment(envir)

	// Get the admin token from the environment variable ISBETMF_ADMIN_TOKEN
	adminToken := os.Getenv("ISBETMF_ADMIN_TOKEN")
	if adminToken == "" {
		// For local testing, use the testing token. For other environments, it is compulsory
		if environment == LOCAL {
			adminToken = "eyJhdWQiOiJodHRwczovL2NhdGFsb2cuaX"
		} else {
			return nil, errl.Errorf("ISBETMF_ADMIN_TOKEN not set for environment %s", environment)
		}
	}

	// Choose the profile from the environment passed
	switch environment {
	case DOME_PRO:
		conf = domeproConfig
		slog.Info("Using the DOME PRO environment")
	case DOME_PRE:
		conf = domepreConfig
		slog.Info("Using the DOME PRE environment")
	case DOME_DEV:
		conf = domedevConfig
		slog.Info("Using the DOME SBX environment")
	case LOCAL:
		conf = lclConfig
		slog.Info("Using the LOCAL environment")
	case ISBE_PRE:
		conf = isbepreConfig
		slog.Info("Using the ISBE PRE environment")
	case ISBE_DEV:
		conf = isbedevConfig
		slog.Info("Using the ISBE DEV environment")
	case ISBE_PRO:
		conf = isbeproConfig
		slog.Info("Using the ISBE PRO environment")
	default:
		conf = lclConfig
		slog.Info("Using the default environment", "environment", environment)
	}

	conf.Debug = debug
	conf.AdminToken = adminToken

	// Check for overrides with environment variables

	proxyEnabled := os.Getenv("ISBETMF_PROXY_ENABLED")
	switch proxyEnabled {
	case "true":
		conf.ProxyEnabled = true
	case "false":
		conf.ProxyEnabled = false
	}
	slog.Info("Proxy", slog.Bool("enabled", conf.ProxyEnabled))

	// Adjust the config according to the proxy status
	if !conf.ProxyEnabled {
		// If there is no proxy, we need to generate the IDs on create
		conf.Features.GenerateIDOnCreate = true
	}

	remoteTMFServer := os.Getenv("ISBETMF_REMOTE_SERVER")
	if remoteTMFServer != "" {
		conf.RemoteTMFServer = remoteTMFServer
	}
	if conf.ProxyEnabled {
		slog.Info("RemoteTMFServer", slog.String("url", conf.RemoteTMFServer))
	}

	verifierServer := os.Getenv("ISBETMF_VERIFIER")
	if verifierServer != "" {
		conf.VerifierServer = verifierServer
	}
	slog.Info("Verifier", slog.String("url", conf.VerifierServer))

	return conf, nil

}

func (c *Config) IsDOME() bool {
	return c.Environment == DOME_PRO || c.Environment == DOME_PRE || c.Environment == DOME_DEV || c.Environment == LOCAL
}

func (c *Config) IsISBE() bool {
	return c.Environment == ISBE_PRE || c.Environment == ISBE_DEV || c.Environment == ISBE_PRO
}
