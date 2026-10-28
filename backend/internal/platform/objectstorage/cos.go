package objectstorage

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	cos "github.com/tencentyun/cos-go-sdk-v5"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

type cosBackend struct {
	sdk *cos.Client
}

func newCOS(p config.StorageProfile, c config.StorageCredentials, transport http.RoundTripper) (backend, error) {
	u, err := url.Parse(p.Endpoint)
	if err != nil {
		return nil, ErrInvalidEndpoint
	}
	sdk := cos.NewClient(&cos.BaseURL{BucketURL: u}, &http.Client{Transport: &cos.AuthorizationTransport{SecretID: c.AccessKey, SecretKey: c.SecretKey, SessionToken: c.SessionToken, Transport: transport}, CheckRedirect: rejectStorageRedirect})
	// An intent owns retries and reconciliation. Do not replay an arbitrary stream
	// or switch hosts inside an SDK after a response was lost.
	sdk.Conf.RetryOpt.Count = 1
	sdk.Conf.RetryOpt.AutoSwitchHost = false
	return cosBackend{sdk: sdk}, nil
}

func rejectStorageRedirect(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }

func (b cosBackend) put(ctx context.Context, key string, reader io.Reader, size int64, contentType, sha string) error {
	meta := http.Header{"X-Cos-Meta-Sha256": []string{sha}}
	condition := http.Header{"X-Cos-Forbid-Overwrite": []string{"true"}}
	resp, err := b.sdk.Object.Put(ctx, key, reader, &cos.ObjectPutOptions{ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{ContentType: contentType, ContentLength: size, XCosMetaXXX: &meta, XOptionHeader: &condition}})
	closeCOS(resp)
	return err
}

func (b cosBackend) stat(ctx context.Context, key string) (ObjectInfo, error) {
	resp, err := b.sdk.Object.Head(ctx, key, nil)
	if err != nil {
		// COS HEAD omits the XML error body. An empty-code 404 cannot
		// distinguish a missing key from a missing bucket, so confirm through
		// GET's authoritative error code. Never reinterpret a denied request.
		var response *cos.ErrorResponse
		if errors.As(err, &response) && response.Response != nil && response.Response.StatusCode == http.StatusNotFound && response.Code == "" {
			proof, proofErr := b.sdk.Object.Get(ctx, key, nil)
			closeCOS(proof)
			if proofErr != nil {
				return ObjectInfo{}, proofErr
			}
		}
		return ObjectInfo{}, err
	}
	defer closeCOS(resp)
	size, err := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	if err != nil || size < 0 {
		return ObjectInfo{}, ErrInvalidObject
	}
	return ObjectInfo{Size: size, ContentType: resp.Header.Get("Content-Type"), SHA256: resp.Header.Get("X-Cos-Meta-Sha256")}, nil
}
func (b cosBackend) get(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := b.sdk.Object.Get(ctx, key, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}
func (b cosBackend) remove(ctx context.Context, key string) error {
	resp, err := b.sdk.Object.Delete(ctx, key)
	closeCOS(resp)
	return err
}
func (b cosBackend) sign(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	u, err := b.sdk.Object.GetPresignedURL2(ctx, http.MethodGet, key, ttl, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
func (b cosBackend) list(ctx context.Context, prefix string, maxItems int) ([]ObjectMetadata, error) {
	var out []ObjectMetadata
	marker := ""
	for {
		page, resp, err := b.sdk.Bucket.Get(ctx, &cos.BucketGetOptions{Prefix: prefix, Marker: marker, MaxKeys: min(maxItems-len(out)+1, 1000)})
		closeCOS(resp)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Contents {
			out = append(out, ObjectMetadata{Key: item.Key, Size: item.Size})
		}
		if len(out) > maxItems {
			return nil, ErrInventoryLimit
		}
		if !page.IsTruncated {
			return out, nil
		}
		if page.NextMarker == "" || page.NextMarker <= marker {
			return nil, ErrInvalidObject
		}
		marker = page.NextMarker
	}
}
func closeCOS(resp *cos.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

func (b cosBackend) ping(ctx context.Context) error { _, err := b.sdk.Bucket.Head(ctx); return err }
