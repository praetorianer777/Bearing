package auth

// The prefixes of the tokens partners hold, so a token found in a log or a repository says what it is and secret
// scanners can find it.
const (
	AccessTokenPrefix   = "bearing_oat_"
	RefreshTokenPrefix  = "bearing_ort_"
	CodePrefix          = "bearing_oac_"
	ClientSecretPrefix  = "bearing_ocs_"
	PersonalTokenPrefix = "bearing_pat_"
)

// NewPrefixedToken is a token behind prefix, and the SHA-256 of the whole of it, which is all the server keeps.
func NewPrefixedToken(prefix string) (token string, hash []byte, err error) {
	raw, _, err := newToken()
	if err != nil {
		return "", nil, err
	}
	token = prefix + raw
	return token, HashToken(token), nil
}

// HashToken is what a token is kept and looked up by.
func HashToken(token string) []byte { return hashToken(token) }
