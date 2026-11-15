package boundaries_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"

	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creation"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationgovernance"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationpreparation"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationreference"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/execution"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/executionwork"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/media"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediaartifact"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediacover"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediapublication"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediareference"
	_ "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/mediawork"
)

// This checks source/descriptor identity across every contract. Field-level
// generation drift is checked separately by regenerating with the pinned tools.
func TestRPCContractsKeepCanonicalDomainNamespaces(t *testing.T) {
	root := filepath.Join("..", "..", "api", "proto")
	packagePattern := regexp.MustCompile(`(?m)^\s*package\s+([a-zA-Z0-9_.]+)\s*;`)
	goPackagePattern := regexp.MustCompile(`(?m)^\s*option\s+go_package\s*=\s*"([^"]+)"\s*;`)
	versionPattern := regexp.MustCompile(`^v[0-9]+$`)
	owners := map[string]bool{"coordination": true, "creation": true, "execution": true, "identity": true, "media": true}
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".proto" {
			return nil
		}
		count++
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name = filepath.ToSlash(name)
		t.Run(name, func(t *testing.T) {
			parts := strings.Split(name, "/")
			if len(parts) != 3 || parts[0] != "glowloom" || !owners[parts[1]] {
				t.Fatalf("contract must have one canonical domain source: %s", name)
			}
			for _, part := range parts {
				if versionPattern.MatchString(part) {
					t.Fatalf("business version path is not canonical: %s", name)
				}
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			packages := packagePattern.FindAllSubmatch(body, -1)
			goPackages := goPackagePattern.FindAllSubmatch(body, -1)
			if len(packages) != 1 || len(goPackages) != 1 {
				t.Fatal("contract must declare exactly one package and go_package")
			}
			wantPackage := "glowloom." + parts[1]
			if string(packages[0][1]) != wantPackage {
				t.Fatalf("package = %s, want %s", packages[0][1], wantPackage)
			}
			goPackage := parts[1]
			// These stateful contracts remain in their wire domain but have
			// separate Go packages to preserve the pure computation closures.
			switch name {
			case "glowloom/creation/account_governance.proto":
				goPackage = "creationgovernance"
			case "glowloom/creation/frozen_input.proto":
				goPackage = "creationhandoff"
			case "glowloom/creation/reference_pin.proto":
				goPackage = "creationreference"
			case "glowloom/creation/project_preparation.proto":
				goPackage = "creationpreparation"
			case "glowloom/execution/project_work.proto":
				goPackage = "executionwork"
			case "glowloom/media/project_cover.proto":
				goPackage = "mediacover"
			case "glowloom/media/project_work.proto":
				goPackage = "mediawork"
			case "glowloom/media/provider_artifact.proto":
				goPackage = "mediaartifact"
			case "glowloom/media/operation_output.proto":
				goPackage = "mediapublication"
			case "glowloom/media/reference_pin.proto":
				goPackage = "mediareference"
			}
			wantGoPackage := "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/" + goPackage + ";" + goPackage
			if string(goPackages[0][1]) != wantGoPackage {
				t.Fatalf("go_package = %s, want %s", goPackages[0][1], wantGoPackage)
			}
			descriptor, err := protoregistry.GlobalFiles.FindFileByPath(name)
			if err != nil {
				t.Fatalf("canonical source has no compiled generated descriptor: %v", err)
			}
			options, ok := descriptor.Options().(*descriptorpb.FileOptions)
			if !ok || options.GetGoPackage() != wantGoPackage || string(descriptor.Package()) != wantPackage {
				t.Fatal("generated descriptor namespace differs from its canonical source")
			}
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("no canonical RPC sources inspected")
	}
}
