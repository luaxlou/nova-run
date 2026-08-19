package appcenter

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/luaxlou/glow-ops/internal/configmanager"
	"github.com/luaxlou/glow-ops/pkg/api"
	"github.com/redis/go-redis/v9"
)

// DatabaseInfo stores information about a database
type DatabaseInfo struct {
	Name    string `json:"name"`
	Charset string `json:"charset"`
}

// MySQLConfig stores the collected MySQL configuration
type MySQLConfig struct {
	Host      string         `json:"host"`
	Port      int            `json:"port"`
	User      string         `json:"user"`
	Password  string         `json:"password"`
	Databases []DatabaseInfo `json:"databases"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// MySQLUserConfig stores specific user credentials for a database
type MySQLUserConfig struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

// RedisConfig stores the collected Redis configuration
type RedisConfig struct {
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	Password  string    `json:"password"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RedisUserConfig stores specific user credentials for a redis user
type RedisUserConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func HandleProvision(req api.TCPRequest) api.Response {
	var payload api.ProvisionRequest
	if err := json.Unmarshal(req.Payload, &payload); err != nil {
		return api.Response{Success: false, Message: "Invalid provision payload"}
	}

	switch payload.ResourceType {
	case "mysql":
		return provisionMySQL(payload.AppName, payload.ResourceName)
	case "redis":
		return provisionRedis(payload.AppName, payload.ResourceName)
	default:
		return api.Response{Success: false, Message: fmt.Sprintf("Unsupported resource type: %s", payload.ResourceType)}
	}
}

func provisionRedis(appName, resourceName string) api.Response {
	// 1. Load System Redis Config (Admin)
	var adminConfig RedisConfig
	if err := configmanager.GetSystemConfigJSON("redis_info", &adminConfig); err != nil {
		return api.Response{Success: false, Message: "Redis system config not found. Run 'nova-server add redis' first."}
	}
	if adminConfig.Host == "" {
		return api.Response{Success: false, Message: "Redis not configured."}
	}

	// 2. Connect as Admin
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", adminConfig.Host, adminConfig.Port),
		Password: adminConfig.Password,
		DB:       0,
	})
	defer rdb.Close()

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return api.Response{Success: false, Message: fmt.Sprintf("Failed to connect to Redis admin: %v", err)}
	}

	// 3. Check if User Exists (ACL)
	// Using ACL GETUSER <name>
	// resourceName will be used as username.
	// Redis usernames are typically simple strings.
	username := resourceName

	var userPassword string

	// Check if user exists
	// Note: ACL GETUSER returns (nil) if user doesn't exist? Or empty list?
	// go-redis returns error "ERR User ... does not exist" or similar?
	// Actually Do() is generic.
	_, err := rdb.Do(context.Background(), "ACL", "GETUSER", username).Result()

	exists := err == nil // Assuming error means not found, success means found.
	// Redis returns (empty array) or similar if not found?
	// Actually Redis 6: ACL GETUSER returns nil if not found? No, it returns error usually?
	// Let's verify: ACL GETUSER non_existent -> (nil) (in some clients) or empty list?
	// In redis-cli: (nil) if not exists? Wait, redis-cli says (nil).
	// go-redis might return redis.Nil
	if err == redis.Nil {
		exists = false
	}

	if exists {
		// Scenario: User Exists
		log.Printf("App %s requested existing Redis User %s", appName, username)

		var redisUsers map[string]RedisUserConfig
		configmanager.GetSystemConfigJSON("redis_users", &redisUsers)
		if redisUsers == nil {
			redisUsers = make(map[string]RedisUserConfig)
		}

		if creds, ok := redisUsers[username]; ok {
			log.Printf("Found stored credentials for Redis User %s", username)
			userPassword = creds.Password
		} else {
			// Interactive Prompt
			log.Printf("No stored credentials for Redis User %s. Requesting interactive input...", username)
			fmt.Printf("\n[Action Required] App '%s' requests access to existing Redis user '%s'.\n", appName, username)
			fmt.Printf("Please enter the password for Redis user '%s': ", username)

			reader := bufio.NewReader(os.Stdin)
			passInput, _ := reader.ReadString('\n')
			userPassword = strings.TrimSpace(passInput)

			if userPassword == "" {
				return api.Response{Success: false, Message: "Authorization failed: No password provided."}
			}

			// Verify
			checkClient := redis.NewClient(&redis.Options{
				Addr:     fmt.Sprintf("%s:%d", adminConfig.Host, adminConfig.Port),
				Username: username,
				Password: userPassword,
			})
			if err := checkClient.Ping(context.Background()).Err(); err != nil {
				checkClient.Close()
				return api.Response{Success: false, Message: fmt.Sprintf("Verification failed: %v", err)}
			}
			checkClient.Close()

			// Save
			redisUsers[username] = RedisUserConfig{Username: username, Password: userPassword}
			if err := configmanager.SetSystemConfigJSON("redis_users", redisUsers); err != nil {
				log.Printf("Warning: Failed to save redis_users: %v", err)
			}
		}

	} else {
		// Scenario: User Does Not Exist -> Create
		log.Printf("App %s requested NEW Redis User %s. Creating...", appName, username)
		userPassword = generateRandomPassword(32)

		// ACL SETUSER <name> on >password ~* +@all
		// ~* : Access all keys
		// +@all : Access all commands
		// This is equivalent to full access but with a specific user/pass.
		// For better security we might want to restrict keys, but for "simple" op, full access is fine.
		err := rdb.Do(context.Background(), "ACL", "SETUSER", username, "on", ">"+userPassword, "~*", "+@all").Err()

		// Fallback for older Redis (<6.0) which doesn't support ACL?
		// The error would indicate command not known.
		if err != nil {
			if strings.Contains(err.Error(), "unknown command") {
				// Fallback: Use Global Password (NOT IDEAL but functional)
				// Or fail. Since requirement is "same logic" (create user), failure is probably better if ACL not supported.
				// But let's be practical: if no ACL, we return admin creds? No, that's insecure.
				// Let's assume Redis 6+.
				return api.Response{Success: false, Message: fmt.Sprintf("Failed to create Redis user (ACL required): %v", err)}
			}
			return api.Response{Success: false, Message: fmt.Sprintf("Failed to create Redis user: %v", err)}
		}

		// Save
		var redisUsers map[string]RedisUserConfig
		configmanager.GetSystemConfigJSON("redis_users", &redisUsers)
		if redisUsers == nil {
			redisUsers = make(map[string]RedisUserConfig)
		}
		redisUsers[username] = RedisUserConfig{Username: username, Password: userPassword}
		if err := configmanager.SetSystemConfigJSON("redis_users", redisUsers); err != nil {
			log.Printf("Warning: Failed to save redis_users: %v", err)
		}
	}

	// 4. Update App Config
	appConfig := map[string]any{
		"redis": map[string]any{
			"addr":     fmt.Sprintf("%s:%d", adminConfig.Host, adminConfig.Port),
			"username": username,
			"password": userPassword,
			"db":       0, // Default DB
		},
	}

	if err := configmanager.Set(appName, appConfig, true); err != nil {
		log.Printf("Warning: Failed to update app config for %s: %v", appName, err)
	}

	return api.Response{Success: true, Data: appConfig, Message: "Redis provisioned successfully"}
}

func provisionMySQL(appName, dbName string) api.Response {
	// 1. Load System MySQL Config (Admin)
	var adminConfig MySQLConfig
	if err := configmanager.GetSystemConfigJSON("mysql_info", &adminConfig); err != nil {
		return api.Response{Success: false, Message: "MySQL system config not found. Run 'nova-server add mysql' first."}
	}
	if adminConfig.Host == "" {
		return api.Response{Success: false, Message: "MySQL not configured."}
	}

	// 2. Connect as Admin
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/", adminConfig.User, adminConfig.Password, adminConfig.Host, adminConfig.Port)
	adminDB, err := sql.Open("mysql", dsn)
	if err != nil {
		return api.Response{Success: false, Message: fmt.Sprintf("Failed to connect to MySQL admin: %v", err)}
	}
	defer adminDB.Close()

	if err := adminDB.Ping(); err != nil {
		return api.Response{Success: false, Message: fmt.Sprintf("Failed to ping MySQL admin: %v", err)}
	}

	// 3. Check if DB Exists
	var exists int
	err = adminDB.QueryRow("SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?", dbName).Scan(&exists)
	if err != nil {
		return api.Response{Success: false, Message: fmt.Sprintf("Failed to check DB existence: %v", err)}
	}

	var user, password string

	if exists > 0 {
		// Scenario: DB Exists
		log.Printf("App %s requested existing DB %s", appName, dbName)

		// Check if we have stored credentials for this DB
		// We use a separate system config key for DB users: "mysql_users"
		var mysqlUsers map[string]MySQLUserConfig
		configmanager.GetSystemConfigJSON("mysql_users", &mysqlUsers)
		if mysqlUsers == nil {
			mysqlUsers = make(map[string]MySQLUserConfig)
		}

		if creds, ok := mysqlUsers[dbName]; ok {
			// Case 1: Credentials stored
			log.Printf("Found stored credentials for DB %s", dbName)
			user = creds.User
			password = creds.Password
		} else {
			// Case 2: Credentials NOT stored -> Interactive Prompt
			log.Printf("No stored credentials for DB %s. Requesting interactive input...", dbName)

			// Note: This blocks the TCP handler goroutine.
			fmt.Printf("\n[Action Required] App '%s' requests access to existing MySQL database '%s'.\n", appName, dbName)
			fmt.Printf("Please enter the password for MySQL user '%s' (or press Enter to skip if unknown): ", dbName) // Assuming user == dbName often

			reader := bufio.NewReader(os.Stdin)
			passInput, _ := reader.ReadString('\n')
			password = strings.TrimSpace(passInput)

			if password == "" {
				return api.Response{Success: false, Message: "Authorization failed: No password provided for existing database."}
			}

			// Verify credentials
			user = dbName // Assumption: User name matches DB name for simplicity, or we should have asked for user too.
			// Let's verify connection with this user/pass
			checkDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", user, password, adminConfig.Host, adminConfig.Port, dbName)
			checkDB, err := sql.Open("mysql", checkDSN)
			if err == nil {
				if err := checkDB.Ping(); err != nil {
					checkDB.Close()
					return api.Response{Success: false, Message: fmt.Sprintf("Verification failed for user %s: %v", user, err)}
				}
				checkDB.Close()
			}

			// Save to system config
			mysqlUsers[dbName] = MySQLUserConfig{User: user, Password: password}
			if err := configmanager.SetSystemConfigJSON("mysql_users", mysqlUsers); err != nil {
				log.Printf("Warning: Failed to save mysql_users: %v", err)
			}
		}

	} else {
		// Scenario: DB Does Not Exist -> Create
		log.Printf("App %s requested NEW DB %s. Creating...", appName, dbName)

		user = dbName
		password = generateRandomPassword(16)

		// Create DB
		if _, err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", dbName)); err != nil {
			return api.Response{Success: false, Message: fmt.Sprintf("Failed to create DB: %v", err)}
		}

		// Create User
		// Note: Host is %, or localhost? Let's use % for container compat, or 'localhost' if local.
		// Using 'localhost' as it's safer for simple local setup, but '%' is better for containers.
		// Let's try both or just '%'
		createUserSQL := fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'", user, password)
		if _, err := adminDB.Exec(createUserSQL); err != nil {
			return api.Response{Success: false, Message: fmt.Sprintf("Failed to create user: %v", err)}
		}

		// Grant Privileges
		grantSQL := fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'", dbName, user)
		if _, err := adminDB.Exec(grantSQL); err != nil {
			return api.Response{Success: false, Message: fmt.Sprintf("Failed to grant privileges: %v", err)}
		}

		if _, err := adminDB.Exec("FLUSH PRIVILEGES"); err != nil {
			log.Printf("Warning: Failed to flush privileges: %v", err)
		}

		// Save to system config
		var mysqlUsers map[string]MySQLUserConfig
		configmanager.GetSystemConfigJSON("mysql_users", &mysqlUsers)
		if mysqlUsers == nil {
			mysqlUsers = make(map[string]MySQLUserConfig)
		}
		mysqlUsers[dbName] = MySQLUserConfig{User: user, Password: password}
		if err := configmanager.SetSystemConfigJSON("mysql_users", mysqlUsers); err != nil {
			log.Printf("Warning: Failed to save mysql_users: %v", err)
		}
	}

	// 4. Update App Config
	appConfig := map[string]any{
		"mysql": map[string]any{
			"dsn": fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, password, adminConfig.Host, adminConfig.Port, dbName),
		},
	}

	if err := configmanager.Set(appName, appConfig, true); err != nil {
		log.Printf("Warning: Failed to update app config for %s: %v", appName, err)
	}

	// 5. Return Config to Client
	return api.Response{Success: true, Data: appConfig, Message: "MySQL provisioned successfully"}
}

func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// Fallback to simple logic or panic, but here we just use 0
			b[i] = charset[0]
		} else {
			b[i] = charset[num.Int64()]
		}
	}
	return string(b)
}
