package cloud

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
)

type ossPublicAccessBlock struct {
	XMLName           xml.Name `xml:"PublicAccessBlockConfiguration"`
	BlockPublicAccess *bool    `xml:"BlockPublicAccess"`
}

func (p *aliyunProvider) GetBucketPublicAccess(ctx context.Context, name string) (cloudservice.BucketPublicAccess, error) {
	bucket, err := p.publicAccessBucket(name)
	if err != nil {
		return cloudservice.BucketPublicAccess{}, err
	}
	res, err := bucket.Do(http.MethodGet, "", map[string]interface{}{"publicAccessBlock": nil}, []oss.Option{oss.WithContext(ctx)}, nil, nil)
	if err != nil {
		var ossErr oss.ServiceError
		if errors.As(err, &ossErr) && ossErr.Code == "NoSuchPublicAccessBlockConfiguration" {
			return cloudservice.BucketPublicAccess{}, nil
		}
		return cloudservice.BucketPublicAccess{}, err
	}
	defer res.Body.Close()
	if err := oss.CheckRespCode(res.StatusCode, []int{http.StatusOK}); err != nil {
		return cloudservice.BucketPublicAccess{}, err
	}
	var config ossPublicAccessBlock
	if err := xml.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&config); err != nil || config.BlockPublicAccess == nil {
		return cloudservice.BucketPublicAccess{}, errors.New("OSS 返回的公共访问配置无效")
	}
	return cloudservice.BucketPublicAccess{BlockPublicAccess: *config.BlockPublicAccess}, nil
}

func (p *aliyunProvider) SetBucketPublicAccess(ctx context.Context, name string, config cloudservice.BucketPublicAccess) error {
	bucket, err := p.publicAccessBucket(name)
	if err != nil {
		return err
	}
	body, err := xml.Marshal(ossPublicAccessBlock{BlockPublicAccess: &config.BlockPublicAccess})
	if err != nil {
		return err
	}
	res, err := bucket.Do(http.MethodPut, "", map[string]interface{}{"publicAccessBlock": nil},
		[]oss.Option{oss.WithContext(ctx), oss.ContentType("application/xml")}, strings.NewReader(string(body)), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return oss.CheckRespCode(res.StatusCode, []int{http.StatusOK})
}

// The V4 signer includes the publicAccessBlock subresource in the canonical request.
func (p *aliyunProvider) publicAccessBucket(name string) (*oss.Bucket, error) {
	client, err := oss.New(p.oss.Config.Endpoint, p.accessKeyID, p.accessKeySecret,
		oss.Region(p.region), oss.AuthVersion(oss.AuthV4), oss.Timeout(10, 60))
	if err != nil {
		return nil, err
	}
	return client.Bucket(name)
}
