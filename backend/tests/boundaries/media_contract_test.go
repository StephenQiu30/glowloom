package boundaries_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/descriptorpb"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/media"
)

func TestMediaContractUsesOneConsistentDomainNamespace(t *testing.T) {
	const source = "glowloom/media/normalization.proto"
	const packageName = "glowloom.media"
	const goPackage = "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/media"
	file := pb.File_glowloom_media_normalization_proto
	if file.Path() != source || string(file.Package()) != packageName {
		t.Fatalf("inconsistent protocol identity: path=%q package=%q", file.Path(), file.Package())
	}
	options, ok := file.Options().(*descriptorpb.FileOptions)
	if !ok || options.GetGoPackage() != goPackage+";media" {
		t.Fatalf("inconsistent generated Go package: %v", options)
	}
	if actual := reflect.TypeFor[pb.NormalizeRequest]().PkgPath(); actual != goPackage {
		t.Fatalf("generated type belongs to %q", actual)
	}
	if pb.Normalization_ServiceDesc.ServiceName != packageName+".Normalization" || pb.Normalization_ServiceDesc.Metadata != source || pb.Normalization_Normalize_FullMethodName != "/"+packageName+".Normalization/Normalize" {
		t.Fatal("generated service, method and source do not share the same domain identity")
	}
	_, caller, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate protocol source")
	}
	content, err := os.ReadFile(filepath.Join(filepath.Dir(caller), "../../api/proto", source))
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range []string{"package " + packageName + ";", "option go_package = \"" + goPackage + ";media\";"} {
		if !strings.Contains(string(content), declaration) {
			t.Fatalf("source and generated contract diverged: missing %s", declaration)
		}
	}
}
