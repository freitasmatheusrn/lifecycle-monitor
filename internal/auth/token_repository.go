package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	refreshTokenPrefix = "refresh_token:"
	tokenFamilyPrefix  = "token_family:"
	userTokensPrefix   = "user_tokens:"
)

type TokenData struct {
	UserID    string    `json:"user_id"`
	FamilyID  string    `json:"family_id"`
	CreatedAt time.Time `json:"created_at"`
	UserAgent string    `json:"user_agent"`
	IP        string    `json:"ip"`
}

type TokenRepository interface {
	StoreToken(ctx context.Context, tokenHash, userID, familyID, userAgent, ip string, ttl time.Duration) error
	GetToken(ctx context.Context, tokenHash string) (*TokenData, error)
	RevokeToken(ctx context.Context, tokenHash string) error
	RevokeTokenFamily(ctx context.Context, familyID string) error
	RevokeAllUserTokens(ctx context.Context, userID string) error
	IsTokenRevoked(ctx context.Context, tokenHash string) (bool, error)
}

type tokenRepository struct {
	client *redis.Client
}

func NewTokenRepository(client *redis.Client) TokenRepository {
	return &tokenRepository{client: client}
}

func (r *tokenRepository) StoreToken(ctx context.Context, tokenHash, userID, familyID, userAgent, ip string, ttl time.Duration) error {
	data := TokenData{
		UserID:    userID,
		FamilyID:  familyID,
		CreatedAt: time.Now(),
		UserAgent: userAgent,
		IP:        ip,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal token data: %w", err)
	}

	pipe := r.client.Pipeline()

	// Store the token data
	tokenKey := refreshTokenPrefix + tokenHash
	pipe.Set(ctx, tokenKey, jsonData, ttl)

	// Add to token family set
	familyKey := tokenFamilyPrefix + familyID
	pipe.SAdd(ctx, familyKey, tokenHash)
	pipe.Expire(ctx, familyKey, ttl)

	// Add to user's token set
	userKey := userTokensPrefix + userID
	pipe.SAdd(ctx, userKey, tokenHash)
	pipe.Expire(ctx, userKey, ttl)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to store token: %w", err)
	}

	return nil
}

func (r *tokenRepository) GetToken(ctx context.Context, tokenHash string) (*TokenData, error) {
	key := refreshTokenPrefix + tokenHash
	data, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	var tokenData TokenData
	if err := json.Unmarshal([]byte(data), &tokenData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token data: %w", err)
	}

	return &tokenData, nil
}

func (r *tokenRepository) RevokeToken(ctx context.Context, tokenHash string) error {
	// First get the token data to find family and user
	tokenData, err := r.GetToken(ctx, tokenHash)
	if err != nil {
		return err
	}
	if tokenData == nil {
		return nil // Token doesn't exist, nothing to revoke
	}

	pipe := r.client.Pipeline()

	// Delete the token
	tokenKey := refreshTokenPrefix + tokenHash
	pipe.Del(ctx, tokenKey)

	// Remove from family set
	familyKey := tokenFamilyPrefix + tokenData.FamilyID
	pipe.SRem(ctx, familyKey, tokenHash)

	// Remove from user's token set
	userKey := userTokensPrefix + tokenData.UserID
	pipe.SRem(ctx, userKey, tokenHash)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	return nil
}

func (r *tokenRepository) RevokeTokenFamily(ctx context.Context, familyID string) error {
	familyKey := tokenFamilyPrefix + familyID

	// Get all tokens in the family
	tokens, err := r.client.SMembers(ctx, familyKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get family tokens: %w", err)
	}

	if len(tokens) == 0 {
		return nil
	}

	pipe := r.client.Pipeline()

	for _, tokenHash := range tokens {
		// Get token data to find user ID
		tokenData, err := r.GetToken(ctx, tokenHash)
		if err != nil {
			continue
		}
		if tokenData != nil {
			// Remove from user's token set
			userKey := userTokensPrefix + tokenData.UserID
			pipe.SRem(ctx, userKey, tokenHash)
		}

		// Delete the token
		tokenKey := refreshTokenPrefix + tokenHash
		pipe.Del(ctx, tokenKey)
	}

	// Delete the family set
	pipe.Del(ctx, familyKey)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to revoke token family: %w", err)
	}

	return nil
}

func (r *tokenRepository) RevokeAllUserTokens(ctx context.Context, userID string) error {
	userKey := userTokensPrefix + userID

	// Get all user's tokens
	tokens, err := r.client.SMembers(ctx, userKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get user tokens: %w", err)
	}

	if len(tokens) == 0 {
		return nil
	}

	// Collect all family IDs to clean up
	familyIDs := make(map[string]bool)

	pipe := r.client.Pipeline()

	for _, tokenHash := range tokens {
		tokenData, err := r.GetToken(ctx, tokenHash)
		if err != nil {
			continue
		}
		if tokenData != nil {
			familyIDs[tokenData.FamilyID] = true
		}

		// Delete the token
		tokenKey := refreshTokenPrefix + tokenHash
		pipe.Del(ctx, tokenKey)
	}

	// Delete the user's token set
	pipe.Del(ctx, userKey)

	// Clean up family sets
	for familyID := range familyIDs {
		familyKey := tokenFamilyPrefix + familyID
		pipe.Del(ctx, familyKey)
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to revoke all user tokens: %w", err)
	}

	return nil
}

func (r *tokenRepository) IsTokenRevoked(ctx context.Context, tokenHash string) (bool, error) {
	key := refreshTokenPrefix + tokenHash
	exists, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check token: %w", err)
	}
	return exists == 0, nil
}
