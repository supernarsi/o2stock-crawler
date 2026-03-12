package service

import (
	"context"
	"errors"
	"testing"

	"o2stock-crawler/internal/db"
	"o2stock-crawler/internal/entity"
)

type mockFeedbackRepo struct {
	createFunc func(ctx context.Context, feedback *entity.Feedback) error
}

func (m *mockFeedbackRepo) Create(ctx context.Context, feedback *entity.Feedback) error {
	return m.createFunc(ctx, feedback)
}

type mockUserRepo struct {
	getByIDFunc func(ctx context.Context, id uint) (*entity.User, error)
}

func (m *mockUserRepo) GetByID(ctx context.Context, id uint) (*entity.User, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, nil
}

// 模拟其他 UserRepositoryInterface 方法以满足接口要求...
func (m *mockUserRepo) GetByOpenID(ctx context.Context, openID string) (*entity.User, error) {
	return nil, nil
}
func (m *mockUserRepo) Create(ctx context.Context, user *entity.User) error { return nil }
func (m *mockUserRepo) Update(ctx context.Context, user *entity.User) error { return nil }
func (m *mockUserRepo) UpdateLoginTime(ctx context.Context, id uint, loginTime interface{}) error {
	return nil
}

func TestSubmitFeedback(t *testing.T) {
	tests := []struct {
		name       string
		uid        uint
		code       string
		content    string
		appVersion string
		ip         []byte
		os         uint8
		mockErr    error
		wantErr    bool
		user       *entity.User
	}{
		{
			name:       "SuccessWithUID",
			uid:        1,
			code:       "",
			content:    "Test content",
			appVersion: "1.0.0",
			ip:         []byte{127, 0, 0, 1},
			os:         1,
			mockErr:    nil,
			wantErr:    false,
			user:       &entity.User{ID: 1, WxOpenID: "db_openid"},
		},
		{
			name:       "EmptyContent",
			uid:        1,
			code:       "",
			content:    "",
			appVersion: "1.0.0",
			ip:         []byte{127, 0, 0, 1},
			os:         1,
			mockErr:    nil,
			wantErr:    true,
		},
		{
			name:       "RepoError",
			uid:        1,
			code:       "",
			content:    "Test content",
			appVersion: "1.0.0",
			ip:         []byte{127, 0, 0, 1},
			os:         1,
			mockErr:    errors.New("db error"),
			wantErr:    true,
			user:       &entity.User{ID: 1, WxOpenID: "db_openid"},
		},
		{
			name:       "SuccessNoUIDAndCode",
			uid:        0,
			code:       "",
			content:    "Anonymous feedback",
			appVersion: "1.0.0",
			ip:         []byte{127, 0, 0, 1},
			os:         1,
			mockErr:    nil,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockFeedbackRepo{
				createFunc: func(ctx context.Context, feedback *entity.Feedback) error {
					expectedOpenID := ""
					if tt.user != nil {
						expectedOpenID = tt.user.WxOpenID
					}
					if tt.uid != feedback.UID || expectedOpenID != feedback.WxOpenID || tt.content != feedback.Content ||
						tt.appVersion != feedback.AppVersion || string(tt.ip) != string(feedback.IP) || tt.os != feedback.OS {
						t.Errorf("unexpected feedback data: %+v", feedback)
					}
					return tt.mockErr
				},
			}
			userRepo := &mockUserRepo{
				getByIDFunc: func(ctx context.Context, id uint) (*entity.User, error) {
					if tt.user != nil {
						return tt.user, nil
					}
					return nil, errors.New("not found")
				},
			}
			dbConfig := &db.Config{}
			s := NewFeedbackService(nil, dbConfig, repo, userRepo)
			err := s.SubmitFeedback(context.Background(), tt.uid, tt.code, tt.content, tt.appVersion, tt.ip, tt.os)
			if (err != nil) != tt.wantErr {
				t.Errorf("SubmitFeedback() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
