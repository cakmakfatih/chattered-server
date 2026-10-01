package models

import (
	"time"

	"github.com/google/uuid"
)

// UserPresenceSession represents one active or recently active client connection.
// A user is online while at least one session has not expired.
type UserPresenceSession struct {
	UserID          uuid.UUID `gorm:"type:uuid;primaryKey;index:idx_user_presence_sessions_user_expiry,priority:1"`
	ConnectionID    string    `gorm:"type:varchar(255);primaryKey"`
	ConnectedAt     time.Time `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	LastHeartbeatAt time.Time `gorm:"type:timestamptz;not null"`
	ExpiresAt       time.Time `gorm:"type:timestamptz;not null;index:idx_user_presence_sessions_user_expiry,priority:2;index:idx_user_presence_sessions_expiry;check:chk_user_presence_sessions_expiry,expires_at > last_heartbeat_at"`

	User User `gorm:"foreignKey:UserID;references:ID"`
}

func (UserPresenceSession) TableName() string {
	return "user_presence_sessions"
}
