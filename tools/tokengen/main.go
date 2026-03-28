// Token Generator Tool
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/egesarisac/SagaWallet/pkg/middleware"
)

func main() {
	defaultSecret := os.Getenv("JWT_SECRET")
	if defaultSecret == "" {
		defaultSecret = "dev-local-jwt-secret-change-me"
	}

	secret := flag.String("secret", defaultSecret, "JWT signing secret (or set JWT_SECRET env var)")
	userID := flag.String("user-id", "150e8400-e29b-41d4-a716-446655440000", "User ID claim")
	rolesCSV := flag.String("roles", "user,admin", "Comma-separated roles")
	expiryHours := flag.Int("expiry-hours", 24, "Token expiry in hours")
	flag.Parse()

	var roles []string
	for _, r := range strings.Split(*rolesCSV, ",") {
		r = strings.TrimSpace(r)
		if r != "" {
			roles = append(roles, r)
		}
	}
	if len(roles) == 0 {
		roles = []string{"user"}
	}

	token, err := middleware.GenerateToken(*secret, *userID, roles, *expiryHours)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("=== JWT Token Generator ===")
	fmt.Printf("User ID: %s\n", *userID)
	fmt.Printf("Roles: %v\n", roles)
	fmt.Printf("Expires in: %d hours\n\n", *expiryHours)
	fmt.Printf("Token:\n%s\n", token)
}
