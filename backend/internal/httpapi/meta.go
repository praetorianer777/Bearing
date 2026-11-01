package httpapi

import (
	"net/http"
	"slices"

	"github.com/praetorianer777/bearing/backend/internal/oauth"
	"github.com/praetorianer777/bearing/backend/internal/version"
)

// Product is how Bearing names itself to partners at /meta.
const Product = "bearing"

// APIVersion is info.version of the OpenAPI document; it rises only when something breaks clients.
const APIVersion = "1"

// capability is one integration feature a partner checks for by name at /meta, with the operations it promises;
// docs/integration.md lists them.
type capability struct {
	name, promise string
	// operations are "METHOD /path" as the operations table spells them.
	operations []string
}

// integrationCapabilities are what this build promises partners. A capability is listed only once every
// operation it names is served.
var integrationCapabilities = []capability{
	{name: oauth.CapabilityPartnerTokens, promise: "An app registers itself, and a person connects it on a consent page by authorization code with PKCE; its grant acts at the MCP endpoint.",
		operations: []string{"POST /oauth/register", "GET /oauth/authorize", "POST /oauth/token", "POST /oauth/revoke"}},
	{name: oauth.CapabilityTokenExchange, promise: "A partner trades a person's realm access token, issued to its client with Bearing among the audiences, for a Bearing grant of theirs (RFC 8693).",
		operations: []string{"POST /oauth/token"}},
	{name: "mcp@1", promise: "An assistant works with the organisation over MCP as the person whose token it holds: POST /mcp, every tool reading.",
		operations: []string{"POST /mcp"}},
	{name: "theme-export@1", promise: "A theme exported and imported as one armature-theme/1 file.",
		operations: []string{"GET /themes/{themeID}/export", "POST /themes/import"}},
}

// capabilities is what this build announces beyond its API version, sorted; a client checks for a name here
// rather than comparing versions.
func capabilities() []string {
	names := make([]string, 0, len(integrationCapabilities))
	for _, c := range integrationCapabilities {
		names = append(names, c.name)
	}
	slices.Sort(names)
	return names
}

type metaResponse struct {
	Product      string       `json:"product"`
	Build        version.Info `json:"build"`
	APIVersion   string       `json:"apiVersion"`
	Capabilities []string     `json:"capabilities"`
}

func (s *Server) handleMeta(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, metaResponse{
		Product:      Product,
		Build:        version.Current(),
		APIVersion:   APIVersion,
		Capabilities: capabilities(),
	})
}
