package cloud

import (
	"context"
	"errors"
	"strings"
)

// BucketPublicAccess controls only the bucket-level block on public permissions.
type BucketPublicAccess struct {
	BlockPublicAccess bool `json:"block_public_access"`
}

type BucketPublicAccessProvider interface {
	GetBucketPublicAccess(context.Context, string) (BucketPublicAccess, error)
	SetBucketPublicAccess(context.Context, string, BucketPublicAccess) error
}

func (s *Service) GetBucketPublicAccess(ctx context.Context, id uint, name, region string) (BucketPublicAccess, error) {
	provider, err := s.bucketPublicAccessProvider(id, name, region)
	if err != nil {
		return BucketPublicAccess{}, err
	}
	return provider.GetBucketPublicAccess(ctx, name)
}

func (s *Service) SetBucketPublicAccess(ctx context.Context, id uint, name, region string, config BucketPublicAccess) error {
	provider, err := s.bucketPublicAccessProvider(id, name, region)
	if err != nil {
		return err
	}
	return provider.SetBucketPublicAccess(ctx, name, config)
}

func (s *Service) bucketPublicAccessProvider(id uint, name, region string) (BucketPublicAccessProvider, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("存储桶名称必填")
	}
	_, storage, err := s.objectStorageProvider(id)
	if err != nil {
		return nil, err
	}
	provider, ok := storage.(BucketPublicAccessProvider)
	if !ok {
		return nil, errors.New("该云提供商暂不支持桶级阻止公共访问配置")
	}
	if err := setStorageRegion(storage, region); err != nil {
		return nil, err
	}
	return provider, nil
}
