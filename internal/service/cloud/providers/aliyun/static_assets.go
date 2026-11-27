package cloud

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
)

func (p *aliyunProvider) SignStaticAssetUpload(bucketName, key, contentType string, size int64, ttl time.Duration) (string, map[string]string, error) {
	now := time.Now().UTC()
	day := now.Format("20060102")
	credential := strings.Join([]string{p.accessKeyID, day, p.region, "oss", "aliyun_v4_request"}, "/")
	fields := map[string]string{
		"key": key, "Content-Type": contentType, "x-oss-object-acl": "private",
		"x-oss-forbid-overwrite": "true", "success_action_status": "200",
		"x-oss-signature-version": "OSS4-HMAC-SHA256", "x-oss-credential": credential,
		"x-oss-date": now.Format("20060102T150405Z"),
	}
	conditions := []any{map[string]string{"bucket": bucketName}, []any{"content-length-range", size, size}}
	for field, value := range fields {
		conditions = append(conditions, map[string]string{field: value})
	}
	policy, err := json.Marshal(map[string]any{"expiration": now.Add(ttl).Format(time.RFC3339), "conditions": conditions})
	if err != nil {
		return "", nil, err
	}
	fields["policy"] = base64.StdEncoding.EncodeToString(policy)
	signingKey := []byte("aliyun_v4" + p.accessKeySecret)
	for _, segment := range []string{day, p.region, "oss", "aliyun_v4_request"} {
		signingKey = ossPolicyHMAC(signingKey, segment)
	}
	fields["x-oss-signature"] = hex.EncodeToString(ossPolicyHMAC(signingKey, fields["policy"]))
	return fmt.Sprintf("https://%s.oss-%s.aliyuncs.com/", bucketName, p.region), fields, nil
}

func ossPolicyHMAC(key []byte, value string) []byte {
	hash := hmac.New(sha256.New, key)
	_, _ = io.WriteString(hash, value)
	return hash.Sum(nil)
}

func (p *aliyunProvider) PublishStaticImage(ctx context.Context, bucketName, source, destination, contentType string, size int64) (string, error) {
	bucket, err := p.oss.Bucket(bucketName)
	if err != nil {
		return "", errors.New("读取静态资源桶失败")
	}
	publicURL := fmt.Sprintf("https://%s.oss-%s.aliyuncs.com/%s", bucketName, p.region, destination)
	if publicStaticImage(ctx, publicURL, contentType, size) {
		return publicURL, nil
	}
	headers, err := bucket.GetObjectDetailedMeta(source, oss.WithContext(ctx))
	etag := headers.Get("ETag")
	actualType, _, _ := mime.ParseMediaType(headers.Get("Content-Type"))
	if err != nil || etag == "" || actualType != contentType || headers.Get("Content-Length") != strconv.FormatInt(size, 10) {
		return "", errors.New("上传对象不存在，或文件类型、大小与上传凭证不一致")
	}
	if err := inspectStaticImage(ctx, bucket, source, etag, contentType); err != nil {
		return "", err
	}
	_, err = bucket.CopyObject(source, destination, oss.WithContext(ctx), oss.CopySourceIfMatch(etag),
		oss.ObjectACL(oss.ACLPublicRead), oss.MetadataDirective(oss.MetaReplace), oss.ContentType(contentType),
		oss.SetHeader("x-oss-forbid-overwrite", "true"))
	if !publicStaticImage(ctx, publicURL, contentType, size) {
		if err != nil {
			log.Printf("发布 OSS 静态资源失败: %v", err)
		}
		return "", errors.New("OSS 直链无法匿名访问，请检查静态资源桶的公共访问限制和对象权限")
	}
	if err := bucket.DeleteObject(source, oss.WithContext(ctx)); err != nil {
		log.Printf("清理 OSS 待发布静态资源失败: bucket=%q key=%q", bucketName, source)
	}
	return publicURL, nil
}

type ossImageInfo struct {
	Format      struct{ Value string }
	ImageWidth  struct{ Value string }
	ImageHeight struct{ Value string }
}

func inspectStaticImage(ctx context.Context, bucket *oss.Bucket, key, etag, contentType string) error {
	invalid := errors.New("OSS 无法识别图片，或宽高超过 1024 像素")
	body, err := bucket.GetObject(key, oss.WithContext(ctx), oss.Process("image/info"), oss.IfMatch(etag))
	if err != nil {
		return invalid
	}
	defer body.Close()
	var image ossImageInfo
	if err := json.NewDecoder(io.LimitReader(body, 16<<10)).Decode(&image); err != nil {
		return invalid
	}
	format := strings.ToLower(image.Format.Value)
	width, _ := strconv.Atoi(image.ImageWidth.Value)
	height, _ := strconv.Atoi(image.ImageHeight.Value)
	if width < 1 || width > cloudservice.MaxStaticImageSide || height < 1 || height > cloudservice.MaxStaticImageSide ||
		!(contentType == "image/png" && format == "png" || contentType == "image/jpeg" && (format == "jpeg" || format == "jpg")) {
		return invalid
	}
	return nil
}

var publicStaticAssetClient = &http.Client{
	Timeout:       8 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

func publicStaticImage(ctx context.Context, publicURL, contentType string, size int64) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, publicURL, nil)
	if err != nil {
		return false
	}
	response, err := publicStaticAssetClient.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	actualType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	return response.StatusCode == http.StatusOK && response.ContentLength == size && actualType == contentType
}
