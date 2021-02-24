package config

var (
	DefaultSecret = "secret"
	DefaultHttpUrl = "http://localhost:8123"
	DefaultListenAddress = "0.0.0.0:8123"
)

type Config struct {
	GithubAccessToken string `long:"token" description:"github access token with full repo permissions"`
	Secret string `long:"secret" description:"webhook secret"`
	HttpUrl string `long:"http-url" description:"http url for invoice delivery"`
	ListenAddress string `long:"listen-address" description:"listen address"`

	LndConnect string `long:"lndconnect" description:"lndconnect string with invoices permissions"`
}

func DefaultConfig() *Config {
	return &Config{
		Secret: DefaultSecret,
		HttpUrl: DefaultHttpUrl,
		ListenAddress: DefaultListenAddress,
	}
}

