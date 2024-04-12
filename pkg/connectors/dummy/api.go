package dummy

type API struct {
	key     string
	secret  string
	baseURL string
}

func NewAPI(key, secret, baseURL string) *API {
	return &API{
		key:     key,
		secret:  secret,
		baseURL: baseURL,
	}
}
