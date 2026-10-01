package models

import (
	"time"

	"github.com/google/uuid"
)

// User stores durable application profile data and privacy preferences.
// Online state is derived from active UserPresenceSession records.
type User struct {
	ID              uuid.UUID  `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	ClerkUserID     string     `gorm:"type:varchar(255);not null;uniqueIndex:ux_users_clerk_user_id"`
	Username        string     `gorm:"type:varchar(30);not null;uniqueIndex:ux_users_username;check:chk_users_username_format,username ~ '^[a-z][a-z0-9_]{2,29}$'"`
	ProfileImageKey *string    `gorm:"type:text"`
	ShowLastSeen    bool       `gorm:"not null;default:true"`
	LastSeenAt      *time.Time `gorm:"type:timestamptz"`
	CreatedAt       time.Time  `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt       time.Time  `gorm:"type:timestamptz;not null;default:CURRENT_TIMESTAMP"`

	PresenceSessions []UserPresenceSession `gorm:"foreignKey:UserID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

func (User) TableName() string {
	return "users"
}
