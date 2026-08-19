package provisioner

import (
	"database/sql"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"

	_ "github.com/go-sql-driver/mysql"
	"github.com/luaxlou/glow-ops/internal/configmanager"
	"github.com/luaxlou/glow-ops/pkg/api"
)

// MySQLConfig stores the MySQL configuration
type MySQLConfig struct {
	Host      string `json:"host"`
	Port      int    `json:"port"`
	User      string `json:"user"`
	Password  string `json:"password"`
	UpdatedAt string `json:"updated_at"`
}

// MySQLUserConfig stores specific user credentials for a database
type MySQLUserConfig struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

// DatabaseInfo stores information about a database
type DatabaseInfo struct {
	Name    string `json:"name"`
	Charset string `json:"charset"`
}

// ProvisionMySQLRequest represents the request to provision MySQL
type ProvisionMySQLRequest struct {
	DBName           string `json:"dbName"`
	Mode             string `json:"mode"` // "create_or_use", "create_only", "use_only"
	ExistingPassword string `json:"existingPassword,omitempty"` // For accessing existing databases
	DryRun           bool   `json:"dryRun,omitempty"` // If true, only plan changes without executing
}

// ProvisionMySQLResult represents the detailed result of MySQL provisioning
type ProvisionMySQLResult struct {
	Action     string `json:"action"` // "created", "reused", "credentials_stored"
	DBName     string `json:"dbName"`
	DSN        string `json:"dsn"`
	ChangeType string `json:"changeType"` // "create", "update", "no_change"
	Message    string `json:"message"`
}

// ProvisionMySQL provisions a MySQL database for an app
func ProvisionMySQL(appName string, req ProvisionMySQLRequest) api.Response {
	// 1. Load System MySQL Config (Admin)
	var adminConfig MySQLConfig
	if err := configmanager.GetSystemConfigJSON("mysql_info", &adminConfig); err != nil {
		return api.Response{Success: false, Message: "MySQL system config not found. Run 'nova-server add mysql' first."}
	}
	if adminConfig.Host == "" {
		return api.Response{Success: false, Message: "MySQL not configured."}
	}

	// 2. Connect as Admin
	adminDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/", adminConfig.User, adminConfig.Password, adminConfig.Host, adminConfig.Port)
	adminDB, err := sql.Open("mysql", adminDSN)
	if err != nil {
		return api.Response{Success: false, Message: fmt.Sprintf("Failed to connect to MySQL admin: %v", err)}
	}
	defer adminDB.Close()

	if err := adminDB.Ping(); err != nil {
		return api.Response{Success: false, Message: fmt.Sprintf("Failed to ping MySQL admin: %v", err)}
	}

	// 3. Check if DB Exists
	var exists int
	err = adminDB.QueryRow("SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?", req.DBName).Scan(&exists)
	if err != nil {
		return api.Response{Success: false, Message: fmt.Sprintf("Failed to check DB existence: %v", err)}
	}

	var user, password string
	result := ProvisionMySQLResult{
		DBName: req.DBName,
	}

	if exists > 0 {
		// Scenario: DB Exists
		log.Printf("App %s requested existing DB %s", appName, req.DBName)

		// Check if we have stored credentials for this DB
		var mysqlUsers map[string]MySQLUserConfig
		configmanager.GetSystemConfigJSON("mysql_users", &mysqlUsers)
		if mysqlUsers == nil {
			mysqlUsers = make(map[string]MySQLUserConfig)
		}

		if creds, ok := mysqlUsers[req.DBName]; ok {
			// Case 1: Credentials stored - reuse
			log.Printf("Found stored credentials for DB %s", req.DBName)
			user = creds.User
			password = creds.Password
			result.Action = "reused"
			result.ChangeType = "no_change"
			result.Message = fmt.Sprintf("Reusing existing database '%s'", req.DBName)
		} else {
			// Case 2: Credentials NOT stored -> Need user to provide them
			log.Printf("No stored credentials for DB %s. Client needs to provide credentials.", req.DBName)

			if req.ExistingPassword == "" {
				// Return structured error indicating credentials are needed
				return api.Response{
					Success: false,
					Message: fmt.Sprintf("Database '%s' already exists but credentials are not stored. Please provide the existing password.", req.DBName),
					Data: map[string]any{
						"error_code": "needs_credentials",
						"db_name":    req.DBName,
						"hint":       "Provide the MySQL user password in the 'existingPassword' field",
					},
				}
			}

			if !req.DryRun {
				// Verify credentials provided by client
				user = req.DBName
				password = req.ExistingPassword

				// Verify connection with this user/pass
				checkDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", user, password, adminConfig.Host, adminConfig.Port, req.DBName)
				checkDB, err := sql.Open("mysql", checkDSN)
				if err != nil {
					return api.Response{Success: false, Message: fmt.Sprintf("Failed to open connection with provided credentials: %v", err)}
				}
				if err := checkDB.Ping(); err != nil {
					checkDB.Close()
					return api.Response{Success: false, Message: fmt.Sprintf("Verification failed for user %s: %v", user, err)}
				}
				checkDB.Close()

				// Save to system config
				mysqlUsers[req.DBName] = MySQLUserConfig{User: user, Password: password}
				if err := configmanager.SetSystemConfigJSON("mysql_users", mysqlUsers); err != nil {
					log.Printf("Warning: Failed to save mysql_users: %v", err)
				}
			}

			result.Action = "credentials_stored"
			result.ChangeType = "update"
			result.Message = fmt.Sprintf("Stored credentials for existing database '%s'", req.DBName)
		}

	} else {
		// Scenario: DB Does Not Exist -> Create
		log.Printf("App %s requested NEW DB %s. Creating...", appName, req.DBName)

		if req.DryRun {
			// Dry run - just return what would happen
			result.Action = "created"
			result.ChangeType = "create"
			result.Message = fmt.Sprintf("Would create new database '%s'", req.DBName)
			result.DSN = fmt.Sprintf("%s:***@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
				req.DBName, adminConfig.Host, adminConfig.Port, req.DBName)

			return api.Response{
				Success: true,
				Message: "Dry run: Database would be created",
				Data: map[string]any{
					"mysql": map[string]any{
						"dbName":     req.DBName,
						"wouldCreate": true,
					},
					"result": result,
				},
			}
		}

		user = req.DBName
		password = generateRandomPassword(16)

		// Create DB
		if _, err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", req.DBName)); err != nil {
			return api.Response{Success: false, Message: fmt.Sprintf("Failed to create DB: %v", err)}
		}

		// Create User
		createUserSQL := fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'", user, password)
		if _, err := adminDB.Exec(createUserSQL); err != nil {
			return api.Response{Success: false, Message: fmt.Sprintf("Failed to create user: %v", err)}
		}

		// Grant Privileges
		grantSQL := fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'", req.DBName, user)
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
		mysqlUsers[req.DBName] = MySQLUserConfig{User: user, Password: password}
		if err := configmanager.SetSystemConfigJSON("mysql_users", mysqlUsers); err != nil {
			log.Printf("Warning: Failed to save mysql_users: %v", err)
		}

		result.Action = "created"
		result.ChangeType = "create"
		result.Message = fmt.Sprintf("Created new database '%s'", req.DBName)
	}

	// 4. Generate DSN
	appDSN := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", user, password, adminConfig.Host, adminConfig.Port, req.DBName)
	result.DSN = appDSN

	// 5. Update App Config (skip in dry run)
	if !req.DryRun {
		appConfig := map[string]any{
			"mysql": map[string]any{
				"dsn": appDSN,
			},
		}

		if err := configmanager.Set(appName, appConfig, true); err != nil {
			log.Printf("Warning: Failed to update app config for %s: %v", appName, err)
		}

		// 6. Return Config with detailed result
		return api.Response{
			Success: true,
			Data: map[string]any{
				"mysql": map[string]any{
					"dsn": appDSN,
				},
				"result": result,
			},
			Message: result.Message,
		}
	}

	// Dry run response
	return api.Response{
		Success: true,
		Data: map[string]any{
			"mysql": map[string]any{
				"dsn": appDSN,
			},
			"result": result,
		},
		Message: "Dry run: " + result.Message,
	}
}

func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			b[i] = charset[0]
		} else {
			b[i] = charset[num.Int64()]
		}
	}
	return string(b)
}
