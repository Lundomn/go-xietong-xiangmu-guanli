package project

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"test.com/project-api/pkg/model"
	common "test.com/project-common"
	"test.com/project-grpc/task"
)

// Simulate a committed attachment whose gRPC acknowledgement is lost. The
// gateway cannot infer a rollback from the error returned by this client.
type committedUploadClient struct {
	task.TaskServiceClient
	resultError error
	committed   *task.TaskFileReqMessage
	saveCalls   int
}

func (client *committedUploadClient) ReadTask(_ context.Context, request *task.TaskReqMessage, _ ...grpc.CallOption) (*task.TaskMessage, error) {
	return &task.TaskMessage{Code: request.TaskCode, ProjectCode: "project-code"}, nil
}

func (client *committedUploadClient) SaveTaskFile(_ context.Context, request *task.TaskFileReqMessage, _ ...grpc.CallOption) (*task.TaskFileResponse, error) {
	client.committed = request
	client.saveCalls++
	if client.resultError != nil {
		return nil, client.resultError
	}
	return &task.TaskFileResponse{}, nil
}

func TestUploadFilesPreservesCommittedFileWhenAcknowledgementIsLost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDirectory); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
	originalClient := TaskServiceClient
	t.Cleanup(func() { TaskServiceClient = originalClient })

	for _, rpcCode := range []codes.Code{codes.DeadlineExceeded, codes.Unavailable} {
		for _, chunkCount := range []int{1, 2} {
			t.Run(rpcCode.String()+"/chunks-"+strconv.Itoa(chunkCount), func(t *testing.T) {
				client := &committedUploadClient{resultError: status.Error(rpcCode, "acknowledgement lost")}
				TaskServiceClient = client
				const content = "firstsecond"
				filename := rpcCode.String() + "-" + strconv.Itoa(chunkCount) + ".txt"
				for chunk := 1; chunk <= chunkCount; chunk++ {
					var body bytes.Buffer
					writer := multipart.NewWriter(&body)
					for name, value := range map[string]string{
						"projectCode": "project-code", "taskCode": "task-code",
						"totalChunks": strconv.Itoa(chunkCount), "chunkNumber": strconv.Itoa(chunk),
						"totalSize": strconv.Itoa(len(content)), "identifier": filename,
						"filename": filename,
					} {
						if err := writer.WriteField(name, value); err != nil {
							t.Fatal(err)
						}
					}
					file, err := writer.CreateFormFile("file", filename)
					if err != nil {
						t.Fatal(err)
					}
					payload := content
					if chunkCount == 2 {
						payload = []string{"first", "second"}[chunk-1]
					}
					if _, err := file.Write([]byte(payload)); err != nil {
						t.Fatal(err)
					}
					if err := writer.Close(); err != nil {
						t.Fatal(err)
					}
					response := httptest.NewRecorder()
					requestContext, _ := gin.CreateTestContext(response)
					requestContext.Request = httptest.NewRequest(http.MethodPost, "/upload", &body)
					requestContext.Request.Header.Set("Content-Type", writer.FormDataContentType())
					requestContext.Set("memberId", int64(1))
					requestContext.Set("organizationCode", "organization-code")
					(&HandlerTask{}).uploadFiles(requestContext)
					var result common.Result
					if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					if chunk < chunkCount {
						if result.Code != 200 || client.saveCalls != 0 {
							t.Fatalf("incomplete upload: result=%+v, save calls=%d", result, client.saveCalls)
						}
					} else if result.Code != common.BusinessCode(rpcCode) {
						t.Fatalf("result code = %d, want original RPC error %d", result.Code, rpcCode)
					}
				}
				if client.saveCalls != 1 || client.committed == nil {
					t.Fatalf("metadata save calls = %d, want one committed attachment", client.saveCalls)
				}
				actual, err := os.ReadFile(filepath.FromSlash(client.committed.PathName))
				if err != nil || string(actual) != content {
					t.Fatalf("committed attachment lost: content=%q, err=%v", actual, err)
				}
			})
		}
	}
}

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
	if content, readErr := os.ReadFile(filepath.Join(directory, identifier+".1.part")); readErr != nil || string(content) != "first" {
		t.Fatalf("failed merge must preserve the existing part for retry: content=%q, err=%v", content, readErr)
	}
}

func TestMergeUploadPartsPreservesPartsWhenPublishFails(t *testing.T) {
	directory := t.TempDir()
	identifier := "upload-id"
	filename := "report.txt"
	for number, content := range []string{"first", "second"} {
		part := filepath.Join(directory, identifier+"."+strconv.Itoa(number+1)+".part")
		if err := os.WriteFile(part, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// A directory at the destination forces the final rename to fail after
	// both parts have been read, without relying on timing or permissions.
	destination := filepath.Join(directory, filename)
	if err := os.Mkdir(destination, 0755); err != nil {
		t.Fatal(err)
	}
	if finalPath, err := mergeUploadParts(directory, identifier, filename, 2); err == nil || finalPath != "" {
		t.Fatalf("merge with a blocked destination = (%q, %v), want a failure", finalPath, err)
	}
	for number, want := range []string{"first", "second"} {
		part := filepath.Join(directory, identifier+"."+strconv.Itoa(number+1)+".part")
		if content, err := os.ReadFile(part); err != nil || string(content) != want {
			t.Fatalf("part %d lost after publish failure: content=%q, err=%v", number+1, content, err)
		}
	}
	if _, err := os.Stat(destination + ".assembling"); !os.IsNotExist(err) {
		t.Fatalf("temporary file survives failed publication: %v", err)
	}
	if err := os.Remove(destination); err != nil {
		t.Fatal(err)
	}
	finalPath, err := mergeUploadParts(directory, identifier, filename, 2)
	if err != nil {
		t.Fatalf("retry without reuploading parts failed: %v", err)
	}
	if content, err := os.ReadFile(finalPath); err != nil || string(content) != "firstsecond" {
		t.Fatalf("retried content = %q, err=%v", content, err)
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
