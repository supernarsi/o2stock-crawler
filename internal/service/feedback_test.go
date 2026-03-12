package service

import (
	"context"
	"errors"
	"testing"

	"o2stock-crawler/internal/entity"
)

type mockFeedbackRepo struct {
	createFunc func(ctx context.Context, feedback *entity.Feedback) error
}

func (m *mockFeedbackRepo) Create(ctx context.Context, feedback *entity.Feedback) error {
	return m.createFunc(ctx, feedback)
}

func TestSubmitFeedback(t *testing.T) {
	tests := []struct {
		name       string
		uid        uint
		openID     string
		content    string
		appVersion string
		ip         []byte
		os         uint8
		mockErr    error
		wantErr    bool
	}{
		{
			name:       "Success",
			uid:        1,
			openID:     "test_openid",
			content:    "Test content",
			appVersion: "1.0.0",
			ip:         []byte{127, 0, 0, 1},
			os:         1,
			mockErr:    nil,
			wantErr:    false,
		},
		{
			name:       "EmptyContent",
			uid:        1,
			openID:     "test_openid",
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
			openID:     "test_openid",
			content:    "Test content",
			appVersion: "1.0.0",
			ip:         []byte{127, 0, 0, 1},
			os:         1,
			mockErr:    errors.New("db error"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockFeedbackRepo{
				createFunc: func(ctx context.Context, feedback *entity.Feedback) error {
					if tt.uid != feedback.UID || tt.openID != feedback.WxOpenID || tt.content != feedback.Content ||
						tt.appVersion != feedback.AppVersion || string(tt.ip) != string(feedback.IP) || tt.os != feedback.OS {
						t.Errorf("unexpected feedback data: %+v", feedback)
					}
					return tt.mockErr
				},
			}
			s := NewFeedbackService(nil, repo)
			err := s.SubmitFeedback(context.Background(), tt.uid, tt.openID, tt.content, tt.appVersion, tt.ip, tt.os)
			if (err != nil) != tt.wantErr {
				t.Errorf("SubmitFeedback() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
