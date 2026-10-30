package service

import (
	_ "embed"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hesusruiz/isbetmf/config"
	"github.com/hesusruiz/isbetmf/internal/errl"
	"github.com/hesusruiz/isbetmf/tmfserver/notifications"
	"github.com/hesusruiz/isbetmf/types"
)

// The DOME implementation has some non-conformances with the TMF specifications, and this is to bypass them.
var DOMEHacks = true

// Request represents a generic HTTP request. Handlers must convert to this representation.
// In this way, we support easily any HTTP framework (currently Fiber), but also other
// future channels like JSON-RPC or even non-HTTP channels like GRPC.
type Request struct {
	Method        string         // The HTTP method (GET, POST, PUT, PATCH, DELETE)
	Action        HttpAction     // The action to perform (READ, CREATE, PUT, UPDATE, DELETE, LIST)
	APIfamily     string         // The API family (e.g., "productCatalogManagement")
	APIVersion    string         // The API version (e.g., "v4", "v5")
	ResourceName  string         // The resource name (e.g., "productOffering", "catalog")
	ID            string         // The ID of the resource (empty for CREATE requests)
	QueryParams   url.Values     // The query parameters (e.g., "limit=1", "offset=0")
	Body          []byte         // The body of the request (empty for READ, LIST, DELETE)
	AuthUser      types.AuthUser // The authenticated user, or zero value if non authenticated
	HealthRequest bool           // Whether this is a health request
}

func (r *Request) ToMap() map[string]any {
	return map[string]any{
		"method":   r.Method,
		"action":   r.Action,
		"api":      r.APIfamily,
		"version":  r.APIVersion,
		"resource": r.ResourceName,
		"id":       r.ID,
	}
}

type HttpAction string

// HttpActionFromMethod converts an HTTP request to an HttpAction
//
//	@param httpMethod the HTTP method (e.g., "GET", "POST", "PUT", "PATCH", "DELETE")
//	@param idParam the ID of the resource (empty for CREATE requests)
func HttpActionFromMethod(httpMethod string, idParam string) HttpAction {
	httpMethod = strings.ToUpper(httpMethod)
	action := HttpActions[httpMethod]
	if idParam == "" && httpMethod == http.MethodGet {
		action = ActionLIST
	}
	return action
}

// These are the possible values for Action
const (
	ActionREAD    HttpAction = "READ"
	ActionCREATE  HttpAction = "CREATE"
	ActionREPLACE HttpAction = "REPLACE"
	ActionUPDATE  HttpAction = "UPDATE"
	ActionDELETE  HttpAction = "DELETE"
	ActionLIST    HttpAction = "LIST"
)

// These are more friendly names for the writers of policy rules and can be used interchangeably
var HttpActions = map[string]HttpAction{
	"GET":    ActionREAD,
	"POST":   ActionCREATE,
	"PUT":    ActionREPLACE,
	"PATCH":  ActionUPDATE,
	"DELETE": ActionDELETE,
	"LIST":   ActionLIST,
}

// Response represents a generic HTTP response.
type Response struct {
	StatusCode int
	Headers    map[string]string
	Body       any
}

type ServerOperatorInfo struct {
	OrganizationIdentifier string
	Did                    string
	Name                   string
	Country                string
	EmailAddress           string
}

// Service is the service for the API.
type Service struct {

	// The environment where we are running
	environment config.Environment

	// The logging level
	logLevel int

	// The admin token used to authenticate the superadmin. Handle as a secret.
	adminToken string

	// Pluggable storage backend
	storage TMFStorer

	// Pluggable PDP interface
	ruleEngine Authorizer

	// The OpenID configuration to use the Verifier Server
	oid *OpenIDConfig

	// Notifications manager
	notif *notifications.Manager

	// TMF Client for proxying requests
	tmfClient *TMFClient

	// Fressness for local objects when proxy enabled
	fressness time.Duration

	// Flag to enable/disable proxy functionality.
	// When not enabled, the service is a standard TMF Server, local only.
	// When enabled, the service is a proxy to a remote TMF Server.
	proxyEnabled bool

	// Information about us (the server operator)
	ServerOperatorOrganizationIdentifier string
	ServerOperatorDid                    string
	ServerOperatorName                   string
	ServerOperatorCountry                string
	ServerEmailAddress                   string

	AdditionalTrustedparties []ServerOperatorInfo

	// The domain of the remote TMForum API server when we act as proxy
	RemoteTMFServer string

	// The power required by a caller to be considered LEAR
	LEARPower types.OnePower

	// The powers required by a caller to be able to create, update and delete a product
	ProductCreatePower types.OnePower
	ProductUpdatePower types.OnePower
	ProductDeletePower types.OnePower

	// The features of the environment
	Features config.Features
}

// NewTMFService creates a new service.
func NewTMFService(cnf *config.Config, storage TMFStorer, ruleEngine Authorizer) (*Service, error) {

	// Parse the YAML definition
	types.ParseActionDefinitions()

	svc := &Service{}

	svc.environment = cnf.Environment
	svc.adminToken = cnf.AdminToken
	svc.storage = storage
	svc.ruleEngine = ruleEngine
	svc.Features = cnf.Features

	// Information about us (the server operator)
	svc.ServerOperatorOrganizationIdentifier = cnf.ServerOperatorOrganizationIdentifier
	svc.ServerOperatorDid = cnf.ServerOperatorDid
	svc.ServerOperatorName = cnf.ServerOperatorName
	svc.ServerOperatorCountry = cnf.ServerOperatorCountry
	svc.ServerEmailAddress = cnf.ServerEmailAddress
	svc.LEARPower = cnf.LEARPower
	svc.ProductCreatePower = cnf.ProductCreatePower
	svc.ProductUpdatePower = cnf.ProductUpdatePower
	svc.ProductDeletePower = cnf.ProductDeletePower

	// The remote TMF server when we act as a proxy to it
	if cnf.ProxyEnabled {
		cnf.RemoteTMFServer = strings.TrimRight(cnf.RemoteTMFServer, "/")
		if cnf.RemoteTMFServer == "" {
			return nil, errl.Errorf("remote TMF server not set")
		}
		svc.RemoteTMFServer = cnf.RemoteTMFServer
		svc.proxyEnabled = cnf.ProxyEnabled

		tmfClientConfig := &TMFClientConfig{
			BaseURL: svc.RemoteTMFServer,
			Timeout: 120,
		}

		svc.tmfClient = NewClient(tmfClientConfig)

		svc.fressness = cnf.ClonePeriod
		if svc.fressness == 0 {
			svc.fressness = config.DefaultClonePeriod
		}
	}

	// Retrieve the OpenId configuration of the Verifier server
	oid, err := NewOpenIDConfig(cnf.VerifierServer)
	if err != nil {
		return nil, errl.Errorf("failed to retrieve OpenID configuration: %w", err)
	}
	svc.oid = oid

	// Initialize notifications with in-memory store and HTTP delivery
	store := notifications.NewMemoryStore()
	deliver := notifications.NewHTTPDelivery(5 * time.Second)
	svc.notif = notifications.NewManager(store, deliver)

	svc.SetLogLevel(3)

	return svc, nil
}

func (s *Service) AdminToken() string {
	return s.adminToken
}

func (s *Service) Environment() config.Environment {
	return s.environment
}

func (s *Service) LogLevel() int {
	return s.logLevel
}

func (s *Service) SetLogLevel(logLevel int) {
	s.logLevel = logLevel
}

func (s *Service) IsDOME() bool {
	return s.environment == config.DOME_PRO || s.environment == config.DOME_PRE || s.environment == config.DOME_DEV || s.environment == config.LOCAL
}

func (s *Service) IsISBE() bool {
	return s.environment == config.ISBE_PRE || s.environment == config.ISBE_DEV || s.environment == config.ISBE_PRO
}

// ToKebabCase converts a camelCase string to kebab-case.
// For example: "productOffering" becomes "product-offering".
func ToKebabCase(s string) string {
	var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
	var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

	snake := matchFirstCap.ReplaceAllString(s, "${1}-${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}-${2}")
	return strings.ToLower(snake)
}

// LifecycleStatusMandatory defines which TMF resources require the lifecycleStatus field as mandatory.
// The map key is the resource name, and the value is the default lifecycleStatus value to set if missing.
// The keys are in lowercase to facilitate case-independent lookup.
var LifecycleStatusMandatory = map[string]string{
	// TMF620
	"catalog":              "In Study",
	"category":             "In Study",
	"productOffering":      "In Study",
	"productOfferingPrice": "In Study",
	"productSpecification": "In Study",

	// TMF633
	"serviceCandidate":     "In Study",
	"serviceCatalog":       "In Study",
	"serviceCategory":      "In Study",
	"serviceSpecification": "In Study",

	// TMF634
	"LogicalResourceSpecification":  "In Study",
	"PhysicalResourceSpecification": "In Study",
	"ResourceCandidate":             "In Study",
	"ResourceCatalog":               "In Study",
	"ResourceCategory":              "In Study",
	"ResourceFunctionSpecification": "In Study",
	"ResourceSpecification":         "In Study",

	// TMF635
	"UsageSpecification": "In Study",

	// TMF651
	"AgreementSpecification": "In Study",
}
