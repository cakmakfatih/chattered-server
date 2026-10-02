package database

import (
	"context"
	"errors"

	"github.com/cakmakfatih/chattered-server/internal/database/models"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (repository *UserRepository) GetByClerkID(ctx context.Context, clerkID string) (*models.User, error) {
	var user models.User
	err := repository.db.WithContext(ctx).Where("clerk_user_id = ?", clerkID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (repository *UserRepository) UsernameExists(ctx context.Context, username string) (bool, error) {
	var count int64
	err := repository.db.WithContext(ctx).Model(&models.User{}).Where("username = ?", username).Count(&count).Error
	return count > 0, err
}

func (repository *UserRepository) Create(ctx context.Context, user *models.User) error {
	return repository.db.WithContext(ctx).Create(user).Error
}
