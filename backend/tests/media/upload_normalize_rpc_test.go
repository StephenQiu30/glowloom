package media_test

import (
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"

	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/media"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/processing/adapter/ffmpeg"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/processing/adapter/grpcserver"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func remoteUploadNormalizer(t *testing.T, clientToken string) mediaapp.UploadNormalizer {
	t.Helper()
	serverToken := strings.Repeat("media-test-identity-", 3)
	handler, err := grpcserver.New(ffmpeg.Normalizer{}, serverToken, 1)
	if err != nil {
		t.Fatal(err)
	}
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.MaxRecvMsgSize(grpcserver.MaxMessageBytes), grpc.MaxSendMsgSize(grpcserver.MaxMessageBytes))
	pb.RegisterNormalizationServer(server, handler)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) && !errors.Is(err, net.ErrClosed) {
			t.Errorf("serve media: %v", err)
		}
	})
	if clientToken == "" {
		clientToken = serverToken
	}
	normalizer, err := mediawire.ProvideUploadNormalizer(config.Config{Env: "local", MediaNormalizationAddress: listener.Addr().String(), MediaServiceToken: clientToken})
	if err != nil {
		t.Fatal(err)
	}
	return normalizer
}

func TestRemoteNormalizationCompletesRealUploadWithoutSharingFiles(t *testing.T) {
	for _, codec := range []string{"libvpx", "libvpx-vp9"} {
		t.Run(codec, func(t *testing.T) {
			normalizer := remoteUploadNormalizer(t, "")
			repo, objects := &uploadRepoFake{}, &uploadObjectsFake{}
			in := uploadInput(t)
			in.File = readUploadWebM(t, uploadWebMFixture(t, codec, "0.6"))
			in.Request.FileName = "远端白膜镜头.webm"
			service := unitRoutedUploadService(repo, nil, mediaflow.FFUploadProber{}, normalizer, mediaflow.FFUploadRenderer{}, objects)
			result, err := fixtureProjectUpload(t.Context(), service, in)
			if err != nil {
				t.Fatal(err)
			}
			var review mediaapp.LocalUploadReview
			if err := json.Unmarshal(repo.asset.ModerationDetail, &review); err != nil {
				t.Fatal(err)
			}
			if result.Asset.Kind != "video" || repo.commits != 1 || repo.asset.MimeType != "video/mp4" || repo.asset.FileName != "远端白膜镜头.mp4" || len(repo.rends) != 2 || len(objects.items) != 3 || review.Normalization == nil || review.Normalization.Source.SHA256 != in.File.SHA256 || repo.asset.SHA256 == nil || review.Normalization.Canonical.SHA256 != *repo.asset.SHA256 || *repo.asset.SHA256 == in.File.SHA256 {
				t.Fatalf("remote upload not committed with canonical lineage: result=%+v asset=%+v review=%+v", result, repo.asset, review)
			}
			if _, err := in.File.File.Stat(); err != nil {
				t.Fatalf("caller source closed: %v", err)
			}
		})
	}
}

func TestRemoteNormalizationFailureNeverPublishesOrFallsBack(t *testing.T) {
	for _, mode := range []string{"disabled", "bad identity"} {
		t.Run(mode, func(t *testing.T) {
			var normalizer mediaapp.UploadNormalizer
			if mode == "disabled" {
				var err error
				normalizer, err = mediawire.ProvideUploadNormalizer(config.Config{})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				normalizer = remoteUploadNormalizer(t, strings.Repeat("wrong-media-identity-", 3))
			}
			repo, objects := &uploadRepoFake{}, &uploadObjectsFake{}
			in := uploadInput(t)
			in.File = readUploadWebM(t, uploadWebMFixture(t, "libvpx", "0.3"))
			in.Request.FileName = "recording.webm"
			service := unitRoutedUploadService(repo, nil, mediaflow.FFUploadProber{}, normalizer, mediaflow.FFUploadRenderer{}, objects)
			if _, err := fixtureProjectUpload(t.Context(), service, in); !errors.Is(err, mediaapp.ErrUnavailable) || repo.commits != 0 || len(objects.items) != 0 {
				t.Fatalf("failed remote conversion published: commits=%d objects=%d error=%v", repo.commits, len(objects.items), err)
			}
		})
	}
}
