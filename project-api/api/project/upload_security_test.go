package project

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"test.com/project-api/pkg/model"
)

func TestSafeUploadPart(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "normal", value: "report.txt", valid: true},
		{name: "unicode", value: "周报-第一版.md", valid: true},
		{name: "empty", value: "", valid: false},
		{name: "parent", value: "..", valid: false},
		{name: "slash", value: "../report.txt", valid: false},
		{name: "windows slash", value: `..\report.txt`, valid: false},
		{name: "drive path", value: `C:\report.txt`, valid: false},
		{name: "control", value: "report\n.txt", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := safeUploadPart(tt.value); got != tt.valid {
				t.Fatalf("safeUploadPart(%q) = %v, want %v", tt.value, got, tt.valid)
			}
		})
	}
}

func TestValidateUploadRequest(t *testing.T) {
	valid := model.UploadFileReq{
		ProjectCode: "project-code",
		TaskCode:    "task-code",
		TotalChunks: 2,
		ChunkNumber: 1,
		TotalSize:   12,
		Identifier:  "upload-id",
	}
	if err := validateUploadRequest(valid); err != nil {
		t.Fatalf("valid upload request rejected: %v", err)
	}

	invalidCases := []model.UploadFileReq{
		{ProjectCode: "project-code", TaskCode: "task-code", TotalChunks: 0, ChunkNumber: 1},
		{ProjectCode: "project-code", TaskCode: "task-code", TotalChunks: 2, ChunkNumber: 3, Identifier: "id"},
		{ProjectCode: "../project", TaskCode: "task-code", TotalChunks: 1, ChunkNumber: 1},
		{ProjectCode: "project-code", TaskCode: "task-code", TotalChunks: 2, ChunkNumber: 1, Identifier: "../id"},
		{ProjectCode: "project-code", TaskCode: "task-code", TotalChunks: 1, ChunkNumber: 1, TotalSize: int(maxUploadSize + 1)},
	}
	for index, request := range invalidCases {
		if err := validateUploadRequest(request); err == nil {
			t.Fatalf("invalid upload request %d was accepted: %+v", index, request)
		}
	}
}

func TestMergeUploadPartsPreservesChunkOrderAndCleansParts(t *testing.T) {
	directory := t.TempDir()
	identifier := "upload-id"
	filename := "report.txt"
	parts := map[int]string{1: "first\n", 2: "second\n", 3: "third\n"}
	for number, content := range parts {
		path := filepath.Join(directory, identifier+"."+strconv.Itoa(number)+".part")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	finalPath, err := mergeUploadParts(directory, identifier, filename, 3)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(content), "first\nsecond\nthird\n"; got != want {
		t.Fatalf("merged content = %q, want %q", got, want)
	}
	if _, err := os.Stat(finalPath + ".assembling"); !os.IsNotExist(err) {
		t.Fatalf("temporary assembling file still exists: %v", err)
	}
	for number := range parts {
		partPath := filepath.Join(directory, identifier+"."+strconv.Itoa(number)+".part")
		if _, err := os.Stat(partPath); !os.IsNotExist(err) {
			t.Fatalf("part %s was not cleaned up: %v", partPath, err)
		}
	}
}

func TestMergeUploadPartsCleansTemporaryFileWhenPartIsMissing(t *testing.T) {
	directory := t.TempDir()
	identifier := "upload-id"
	filename := "report.txt"
	if err := os.WriteFile(filepath.Join(directory, identifier+".1.part"), []byte("first"), 0644); err != nil {
		t.Fatal(err)
	}

	finalPath, err := mergeUploadParts(directory, identifier, filename, 2)
	if err == nil {
		t.Fatal("mergeUploadParts succeeded with a missing part")
	}
	if finalPath != "" {
		t.Fatalf("final path = %q, want empty path on failure", finalPath)
	}
	if _, statErr := os.Stat(filepath.Join(directory, filename+".assembling")); !os.IsNotExist(statErr) {
		t.Fatalf("temporary assembling file was not removed: %v", statErr)
	}
}

func TestDownloadFileRejectsUnsafePathBeforeServiceCall(t *testing.T) {
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Params = gin.Params{{Key: "filepath", Value: "/project-code/task-code/2026-01-01/../secret.txt"}}

	(&HandlerTask{}).downloadFile(context)
	if response.Code != 400 {
		t.Fatalf("status = %d, want 400", response.Code)
	}
}
