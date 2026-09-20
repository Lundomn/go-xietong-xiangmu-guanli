package code_gen

import (
	"os"
	"testing"
)

func TestGenStruct(t *testing.T) {
	if os.Getenv("RUN_CODE_GEN_TEST") != "1" {
		t.Skip("code generator requires an explicitly configured MySQL instance")
	}
	//GenStruct("ms_project", "Project")
	GenProtoMessage("ms_project", "ProjectMessage")
}
