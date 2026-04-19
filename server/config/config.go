package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server       ServerConfig         `mapstructure:"server"`
	Database     DatabaseConfig       `mapstructure:"mysql"`
	Redis        RedisSettings        `mapstructure:"redis"`
	Zap          ZapConfig            `mapstructure:"zap"`
	JWT          JWTSettings          `mapstructure:"jwt"`
	COS          COSConfig            `mapstructure:"cos"`
	Kafka        KafkaSettings        `mapstructure:"kafka"`
	DueScheduler DueSchedulerSettings `mapstructure:"due-scheduler"`
	Email        EmailConfig          `mapstructure:"email"`
}

type EmailConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	From     string `mapstructure:"from"`
}

type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Mode string `mapstructure:"mode"`
}

type DatabaseConfig struct {
	Host            string        `mapstructure:"path"`
	Port            int           `mapstructure:"port"`
	DBName          string        `mapstructure:"db-name"`
	Username        string        `mapstructure:"username"`
	Password        string        `mapstructure:"password"`
	MaxIdleConns    int           `mapstructure:"max-idle-conns"`
	MaxOpenConns    int           `mapstructure:"max-open-conns"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn-max-idle-time"`
	ConnMaxLifetime time.Duration `mapstructure:"conn-max-lifetime"`
	Charset         string        `mapstructure:"config"`
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
		d.Username, d.Password, d.Host, d.Port, d.DBName, d.Charset)
}

type RedisSettings struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type JWTSettings struct {
	Secret    string        `mapstructure:"secret"`
	Issuer    string        `mapstructure:"issuer"`
	Audience  string        `mapstructure:"audience"`
	AccessTTL time.Duration `mapstructure:"access-ttl"`
}

type COSConfig struct {
	SecretID  string `mapstructure:"secret-id"`
	SecretKey string `mapstructure:"secret-key"`
	Bucket    string `mapstructure:"bucket"`
	Region    string `mapstructure:"region"`
}

type KafkaSettings struct {
	Enable   bool     `mapstructure:"enable"`
	Brokers  []string `mapstructure:"brokers"`
	Topic    string   `mapstructure:"topic"`
	DLQTopic string   `mapstructure:"dlq-topic"`
	GroupID  string   `mapstructure:"group-id"`
	Workers  int      `mapstructure:"workers"`
}

type DueSchedulerSettings struct {
	LocalPollingEnabled bool          `mapstructure:"local-polling-enabled"`
	ScheduleURL         string        `mapstructure:"schedule-url"`
	CancelURL           string        `mapstructure:"cancel-url"`
	CallbackURL         string        `mapstructure:"callback-url"`
	CallbackToken       string        `mapstructure:"callback-token"`
	RequestTimeout      time.Duration `mapstructure:"request-timeout"`
	PingURL             string        `mapstructure:"ping-url"`
}

var GlobalConfig *Config

func LoadConfig() (*Config, error) {
	v := viper.New()
	if p := strings.TrimSpace(os.Getenv("TODO_CONFIG_FILE")); p != "" {
		v.SetConfigFile(p)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yml")
		v.AddConfigPath(".")
		v.AddConfigPath("./server")
		v.AddConfigPath("./..")
	}

	// Read from environment variables matching TODO_*
	v.SetEnvPrefix("TODO")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()
	bindEnvs(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config failed: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config failed: %w", err)
	}

	setDefaults(&cfg)
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	GlobalConfig = &cfg
	return &cfg, nil
}

func bindEnvs(v *viper.Viper) {
	keys := []string{
		"server.port", "server.mode",
		"jwt.secret", "jwt.issuer", "jwt.audience", "jwt.access-ttl",
		"redis.addr", "redis.password", "redis.db",
		"mysql.path", "mysql.port", "mysql.config", "mysql.db-name", "mysql.username", "mysql.password",
		"mysql.max-idle-conns", "mysql.max-open-conns", "mysql.conn-max-idle-time", "mysql.conn-max-lifetime",
		"kafka.enable", "kafka.brokers", "kafka.topic", "kafka.dlq-topic", "kafka.group-id", "kafka.workers",
		"due-scheduler.local-polling-enabled", "due-scheduler.schedule-url", "due-scheduler.cancel-url",
		"due-scheduler.callback-url", "due-scheduler.callback-token", "due-scheduler.request-timeout", "due-scheduler.ping-url",
		"email.host", "email.port", "email.username", "email.password", "email.from",
		"cos.secret-id", "cos.secret-key", "cos.bucket", "cos.region",
	}
	for _, key := range keys {
		_ = v.BindEnv(key)
	}
}

func validateConfig(cfg *Config) error {
	if !cfg.Kafka.Enable {
		return fmt.Errorf("config error: kafka.enable must be true")
	}
	if len(cfg.Kafka.Brokers) == 0 {
		return fmt.Errorf("config error: kafka.brokers is empty")
	}

	if strings.EqualFold(strings.TrimSpace(cfg.Server.Mode), "release") {
		invalidKeys := make([]string, 0, 4)
		if isPlaceholderValue(cfg.JWT.Secret) {
			invalidKeys = append(invalidKeys, "jwt.secret")
		}
		if isPlaceholderValue(cfg.Database.Password) {
			invalidKeys = append(invalidKeys, "mysql.password")
		}
		if isPlaceholderValue(cfg.Redis.Password) {
			invalidKeys = append(invalidKeys, "redis.password")
		}
		if isPlaceholderValue(cfg.DueScheduler.CallbackToken) {
			invalidKeys = append(invalidKeys, "due-scheduler.callback-token")
		}
		if len(invalidKeys) > 0 {
			return fmt.Errorf("config error: insecure placeholders in release mode: %s", strings.Join(invalidKeys, ", "))
		}
	}

	return nil
}

func isPlaceholderValue(v string) bool {
	value := strings.TrimSpace(strings.ToLower(v))
	switch value {
	case "", "must_set_in_env", "dev-only-secret-please-change-me-32bytes-min!", "secure-token-123", "dev-scheduler-callback-token", "123456", "root":
		return true
	default:
		return false
	}
}

func setDefaults(cfg *Config) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.Mode == "" {
		cfg.Server.Mode = "debug"
	}
	if cfg.JWT.Secret == "" {
		cfg.JWT.Secret = "dev-only-secret-please-change-me-32bytes-min!"
	}
	if cfg.JWT.Issuer == "" {
		cfg.JWT.Issuer = "todo-api"
	}
	if cfg.JWT.Audience == "" {
		cfg.JWT.Audience = "todo-frontend"
	}
	if cfg.JWT.AccessTTL == 0 {
		cfg.JWT.AccessTTL = 24 * time.Hour
	}
	if cfg.Database.MaxIdleConns == 0 {
		cfg.Database.MaxIdleConns = 10
	}
	if cfg.Database.MaxOpenConns == 0 {
		cfg.Database.MaxOpenConns = 100
	}
	if cfg.DueScheduler.RequestTimeout <= 0 {
		cfg.DueScheduler.RequestTimeout = 3 * time.Second
	}
	if cfg.DueScheduler.CallbackToken == "" {
		cfg.DueScheduler.CallbackToken = "dev-scheduler-callback-token"
	}
}
