package application

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

const (
	uploadTTL        = 15 * time.Minute
	maxDocumentBytes = 20 << 20
)

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

var ErrNotFound = errors.New("media record not found")

type Error struct {
	Code, Message, NextAction string
	Status                    int
	Details                   map[string]any
}

func (e *Error) Error() string { return e.Message }

type Actor struct {
	UserID       string
	TokenVersion int
}

type Repository interface {
	Authorize(context.Context, Actor, string, bool) error
	FindUploadByKey(context.Context, string, string, bool) (domain.UploadSession, error)
	GetUpload(context.Context, string, bool) (domain.UploadSession, error)
	CreateUpload(context.Context, domain.UploadSession) error
	SaveUpload(context.Context, domain.UploadSession) error
	CreateCompletion(context.Context, domain.MediaObject, domain.MediaVersion, domain.Task) error
	Completion(context.Context, domain.UploadSession) (domain.MediaObject, domain.MediaVersion, domain.Task, error)
	GetVersion(context.Context, string) (domain.MediaVersion, error)
}

type TransactionManager interface {
	WithinTransaction(context.Context, func(Repository) error) error
}

type ObjectStore interface {
	PresignPut(context.Context, string, time.Duration) (*url.URL, error)
	ReadVerified(context.Context, string, int64, string, int64) ([]byte, error)
}

type Config struct {
	Now   func() time.Time
	NewID func() string
}

type Service struct {
	transactions TransactionManager
	objects      ObjectStore
	config       Config
}

type InitializeCommand struct {
	WorkspaceID, Kind, Filename, MIMEType, SHA256, IdempotencyKey string
	SizeBytes                                                     int64
}

type UploadCapability struct {
	URL, Method string
	Headers     map[string]string
	ExpiresAt   time.Time
}

type Initialization struct {
	Session domain.UploadSession
	Upload  UploadCapability
}

type Completion struct {
	Object  domain.MediaObject
	Version domain.MediaVersion
	Task    domain.Task
}

func NewService(transactions TransactionManager, objects ObjectStore, config Config) *Service {
	return &Service{transactions: transactions, objects: objects, config: config}
}

func (service *Service) Initialize(ctx context.Context, actor Actor, command InitializeCommand) (Initialization, error) {
	command.Filename = strings.TrimSpace(command.Filename)
	command.MIMEType = strings.TrimSpace(command.MIMEType)
	if command.WorkspaceID == "" || command.Kind != "document" || command.Filename == "" || len(command.Filename) > 255 || path.Base(command.Filename) != command.Filename || command.SizeBytes < 1 || command.SizeBytes > maxDocumentBytes || !sha256Pattern.MatchString(command.SHA256) || strings.TrimSpace(command.IdempotencyKey) == "" || len(command.IdempotencyKey) > 200 {
		return Initialization{}, invalid("Invalid upload declaration")
	}
	if !oneOf(command.MIMEType, "text/markdown", "text/plain", "application/vnd.openxmlformats-officedocument.wordprocessingml.document") {
		return Initialization{}, invalid("Unsupported document media type")
	}
	now := service.config.Now().UTC()
	var session domain.UploadSession
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		if err := repo.Authorize(ctx, actor, command.WorkspaceID, true); err != nil {
			return err
		}
		existing, findErr := repo.FindUploadByKey(ctx, command.WorkspaceID, command.IdempotencyKey, true)
		if findErr == nil {
			if !sameDeclaration(existing, command) {
				return &Error{Code: "idempotency_conflict", Message: "Idempotency key was already used with different input", Status: 409}
			}
			if existing.Status != "pending" || !existing.ExpiresAt.After(now) {
				return &Error{Code: "state_conflict", Message: "Upload session is no longer pending", Status: 409, NextAction: "start_new_upload"}
			}
			session = existing
			return nil
		}
		if !errors.Is(findErr, ErrNotFound) {
			return findErr
		}
		session = domain.UploadSession{ID: service.config.NewID(), WorkspaceID: command.WorkspaceID, Status: "pending", Kind: command.Kind, Filename: command.Filename, SizeBytes: command.SizeBytes, MIMEType: command.MIMEType, SHA256: command.SHA256, IdempotencyKey: command.IdempotencyKey, ExpiresAt: now.Add(uploadTTL), CreatedAt: now, UpdatedAt: now}
		session.ObjectKey = fmt.Sprintf("uploads/%s/%s/%s", session.WorkspaceID, session.ID, session.Filename)
		return repo.CreateUpload(ctx, session)
	})
	if err != nil {
		return Initialization{}, err
	}
	presignTTL := session.ExpiresAt.Sub(service.config.Now().UTC())
	if presignTTL <= 0 {
		return Initialization{}, &Error{Code: "state_conflict", Message: "Upload session has expired", Status: 409, NextAction: "start_new_upload"}
	}
	presigned, err := service.objects.PresignPut(ctx, session.ObjectKey, presignTTL)
	if err != nil {
		return Initialization{}, &Error{Code: "dependency_unavailable", Message: "Object storage is unavailable", Status: 503, NextAction: "retry"}
	}
	return Initialization{Session: session, Upload: UploadCapability{URL: presigned.String(), Method: "PUT", Headers: map[string]string{}, ExpiresAt: session.ExpiresAt}}, nil
}

func (service *Service) Complete(ctx context.Context, actor Actor, uploadID string) (Completion, error) {
	var session domain.UploadSession
	var existing *Completion
	now := service.config.Now().UTC()
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		loaded, err := repo.GetUpload(ctx, uploadID, true)
		if errors.Is(err, ErrNotFound) {
			return notFound("Upload session not found")
		}
		if err != nil {
			return err
		}
		if err = repo.Authorize(ctx, actor, loaded.WorkspaceID, true); err != nil {
			return err
		}
		session = loaded
		if session.Status == "completed" {
			object, version, task, completionErr := repo.Completion(ctx, session)
			if completionErr != nil {
				return completionErr
			}
			value := Completion{Object: object, Version: version, Task: task}
			existing = &value
			return nil
		}
		if session.Status != "pending" || !session.ExpiresAt.After(now) {
			return &Error{Code: "state_conflict", Message: "Upload session is not completable", Status: 409, NextAction: "start_new_upload"}
		}
		return nil
	})
	if err != nil || existing != nil {
		if existing != nil {
			return *existing, nil
		}
		return Completion{}, err
	}
	if _, err = service.objects.ReadVerified(ctx, session.ObjectKey, session.SizeBytes, session.SHA256, maxDocumentBytes); err != nil {
		return Completion{}, &Error{Code: "invalid_request", Message: "Uploaded object does not match its declaration", Status: 422, NextAction: "upload_again"}
	}

	var completion Completion
	err = service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		locked, lockErr := repo.GetUpload(ctx, uploadID, true)
		if lockErr != nil {
			return lockErr
		}
		if locked.Status == "completed" {
			completion.Object, completion.Version, completion.Task, lockErr = repo.Completion(ctx, locked)
			return lockErr
		}
		if locked.Status != "pending" || !locked.ExpiresAt.After(now) {
			return &Error{Code: "state_conflict", Message: "Upload session is not completable", Status: 409}
		}
		objectID := service.config.NewID()
		versionID := service.config.NewID()
		taskID := service.config.NewID()
		completion.Object = domain.MediaObject{ID: objectID, WorkspaceID: locked.WorkspaceID, Kind: locked.Kind, SourceType: "upload", Status: "active", CurrentVersionID: &versionID, Revision: 1, CreatedAt: now, UpdatedAt: now}
		completion.Version = domain.MediaVersion{ID: versionID, WorkspaceID: locked.WorkspaceID, MediaObjectID: objectID, VersionNo: 1, Filename: locked.Filename, SHA256: locked.SHA256, SizeBytes: locked.SizeBytes, MIMEType: locked.MIMEType, ObjectKey: locked.ObjectKey, ProbeStatus: "ready", ProbeAttempt: 1, CreatedAt: now, MediaObject: completion.Object}
		completion.Task = domain.Task{ID: taskID, WorkspaceID: locked.WorkspaceID, TaskType: "media_probe", RequestType: "media_version", RequestID: versionID, Scope: []byte(`{"episode_id":null,"render_snapshot_id":null,"usage_type":null,"usage_id":null,"input_version_id":null,"input_hash":null}`), Status: "succeeded", ProgressStage: "ready", CancelStatus: "none", Revision: 1, CreatedAt: now, UpdatedAt: now}
		if lockErr = repo.CreateCompletion(ctx, completion.Object, completion.Version, completion.Task); lockErr != nil {
			return lockErr
		}
		locked.Status = "completed"
		locked.MediaObjectID = &objectID
		locked.CompletedAt = &now
		locked.UpdatedAt = now
		return repo.SaveUpload(ctx, locked)
	})
	return completion, err
}

func (service *Service) GetVersion(ctx context.Context, actor Actor, versionID string) (domain.MediaVersion, error) {
	var version domain.MediaVersion
	err := service.transactions.WithinTransaction(ctx, func(repo Repository) error {
		var err error
		version, err = repo.GetVersion(ctx, versionID)
		if errors.Is(err, ErrNotFound) {
			return notFound("Media version not found")
		}
		if err != nil {
			return err
		}
		return repo.Authorize(ctx, actor, version.WorkspaceID, false)
	})
	return version, err
}

func (service *Service) Content(ctx context.Context, actor Actor, versionID string) (domain.MediaVersion, []byte, error) {
	version, err := service.GetVersion(ctx, actor, versionID)
	if err != nil {
		return domain.MediaVersion{}, nil, err
	}
	contents, err := service.objects.ReadVerified(ctx, version.ObjectKey, version.SizeBytes, version.SHA256, maxDocumentBytes)
	if err != nil {
		return domain.MediaVersion{}, nil, &Error{Code: "dependency_unavailable", Message: "Media content is unavailable", Status: 503, NextAction: "retry"}
	}
	return version, contents, nil
}

func sameDeclaration(session domain.UploadSession, command InitializeCommand) bool {
	return session.Kind == command.Kind && session.Filename == command.Filename && session.SizeBytes == command.SizeBytes && session.MIMEType == command.MIMEType && session.SHA256 == command.SHA256
}

func invalid(message string) error {
	return &Error{Code: "invalid_request", Message: message, Status: 422}
}
func notFound(message string) error { return &Error{Code: "not_found", Message: message, Status: 404} }
func oneOf(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}
