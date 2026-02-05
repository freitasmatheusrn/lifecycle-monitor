package configs

import "github.com/spf13/viper"

type Configs struct {
	DBDriver           string `mapstructure:"DB_DRIVER"`
	DBHost             string `mapstructure:"DB_HOST"`
	DBName             string `mapstructure:"DB_NAME"`
	DBPort             string `mapstructure:"DB_PORT"`
	DBUser             string `mapstructure:"DB_USER"`
	DBPassword         string `mapstructure:"DB_PASSWORD"`
	WebServerPort      string `mapstructure:"WEB_SERVER_PORT"`
	JWTSecret          string `mapstructure:"JWT_SECRET"`
	AccessTokenExp     int    `mapstructure:"ACCESS_TOKEN_EXP"`  // Default: 900 (15 min)
	RefreshTokenExp    int    `mapstructure:"REFRESH_TOKEN_EXP"` // Default: 604800 (7 days)
	RedisHost          string `mapstructure:"REDIS_HOST"`
	RedisPort          string `mapstructure:"REDIS_PORT"`
	RedisPassword      string `mapstructure:"REDIS_PASSWORD"`
	RedisDB            int    `mapstructure:"REDIS_DB"`
	TwilioAccountSID   string `mapstructure:"TWILIO_ACCOUNT_SID"`
	TwilioAuthToken    string `mapstructure:"TWILIO_AUTH_TOKEN"`
	TwilioApiKey       string `mapstructure:"TWILIO_API_KEY"`
	TwilioApiSecret    string `mapstructure:"TWILIO_API_SECRET"`
	TwilioNumber       string `mapstructure:"TWILIO_NUMBER"`
	SIEMENS_URL        string `mapstructure:"SIEMENS_URL"`
	MAILJET_API_KEY    string `mapstructure:"MAILJET_API_KEY"`
	MAILJET_API_SECRET string `mapstructure:"MAILJET_API_SECRET"`
	SMTP_HOST          string `mapstructure:"SMTP_HOST"`
	SMTP_PORT          int `mapstructure:"SMTP_PORT"`
	SMTP_USER          string `mapstructure:"SMTP_USER"`
	SMTP_PASS          string `mapstructure:"SMTP_PASS"`
	CronExpression     string   `mapstructure:"CRON_EXPRESSION"` // Cron expression for lifecycle update job (6 fields with seconds)
	LogPath            string   `mapstructure:"LOG_PATH"`        // Path to log file (e.g., "/var/log/scheduler.log")
	AlertRecipients    []string `mapstructure:"ALERT_RECIPIENTS"` // Email recipients for error alerts
}

func LoadConfig(path string) (*Configs, error) {
	var cfg *Configs
	viper.SetConfigName("app_config")
	viper.SetConfigType("env")
	viper.SetConfigFile(".env")
	viper.AutomaticEnv()

	// Set defaults for token expiration
	viper.SetDefault("ACCESS_TOKEN_EXP", 900)     // 15 minutes
	viper.SetDefault("REFRESH_TOKEN_EXP", 604800) // 7 days

	// Set defaults for Redis
	viper.SetDefault("REDIS_HOST", "localhost")
	viper.SetDefault("REDIS_PORT", "6379")
	viper.SetDefault("REDIS_PASSWORD", "")
	viper.SetDefault("REDIS_DB", 0)

	// Set default for cron expression (runs at 3:00 AM every day)
	viper.SetDefault("CRON_EXPRESSION", "0 0 3 * * *")

	// Set default for log path (empty means stdout only)
	viper.SetDefault("LOG_PATH", "")

	// Set default for alert recipients (empty means no alerts)
	viper.SetDefault("ALERT_RECIPIENTS", []string{"freitasmatheus@lunaltas.com"})

	err := viper.ReadInConfig()
	if err != nil {
		panic(err)
	}
	err = viper.Unmarshal(&cfg)
	if err != nil {
		panic(err)
	}

	return cfg, nil
}
