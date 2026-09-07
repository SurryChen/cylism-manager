package main

import "testing"

func TestValidateSecurityConfig(t *testing.T) {
	validKey := "01234567890123456789012345678901"

	tests := []struct {
		name          string
		encryptionKey string
		jwtSecret     string
		adminPassword string
		wantErr       bool
	}{
		{
			name:          "accepts configured credentials",
			encryptionKey: validKey,
			jwtSecret:     "jwt-signing-secret",
			adminPassword: "admin-password",
		},
		{
			name:          "rejects invalid encryption key length",
			encryptionKey: "short",
			jwtSecret:     "jwt-signing-secret",
			adminPassword: "admin-password",
			wantErr:       true,
		},
		{
			name:          "rejects empty JWT secret",
			encryptionKey: validKey,
			adminPassword: "admin-password",
			wantErr:       true,
		},
		{
			name:          "rejects empty admin password",
			encryptionKey: validKey,
			jwtSecret:     "jwt-signing-secret",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSecurityConfig(tt.encryptionKey, tt.jwtSecret, tt.adminPassword)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSecurityConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
