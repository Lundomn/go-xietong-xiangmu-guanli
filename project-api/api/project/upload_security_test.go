package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/iotest"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"test.com/project-api/pkg/model"
	common "test.com/project-common"
	"test.com/project-common/tms"
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

func uploadChunkForTest(t *testing.T, fields map[string]string, content string, memberID int64, organization string) common.Result {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	file, err := writer.CreateFormFile("file", fields["filename"])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/upload", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Set("memberId", memberID)
	c.Set("organizationCode", organization)
	(&HandlerTask{}).uploadFiles(c)
	var result common.Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestUploadChunkReplayCannotReplaceAcceptedContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	previousClient := TaskServiceClient
	t.Cleanup(func() { TaskServiceClient = previousClient })
	client := &committedUploadClient{}
	TaskServiceClient = client
	fields := map[string]string{
		"projectCode": "project-code", "taskCode": "task-code", "filename": "replay.txt",
		"identifier": "replay", "totalChunks": "2", "chunkNumber": "1", "totalSize": "6",
	}
	if result := uploadChunkForTest(t, fields, "old", 1, "organization-code"); result.Code != 200 {
		t.Fatalf("first chunk rejected: %+v", result)
	}
	if result := uploadChunkForTest(t, fields, "old", 1, "organization-code"); result.Code != 200 {
		t.Fatalf("identical replay rejected: %+v", result)
	}
	if result := uploadChunkForTest(t, fields, "bad", 1, "organization-code"); result.Code != 409 {
		t.Errorf("different content for accepted chunk = %+v, want conflict", result)
	}
	part := uploadPartPath(filepath.Join("upload", "project-code", "task-code", tms.FormatYMD(time.Now())), "replay", 1)
	if got, err := os.ReadFile(part); err != nil || string(got) != "old" {
		t.Errorf("accepted chunk replaced: %q %v", got, err)
	}
	if client.saveCalls != 0 {
		t.Errorf("incomplete upload saved metadata %d times", client.saveCalls)
	}
	fields["chunkNumber"] = "2"
	if result := uploadChunkForTest(t, fields, "new", 1, "organization-code"); result.Code != 200 {
		t.Fatalf("original upload did not complete: %+v", result)
	}
	if client.saveCalls != 1 || client.committed == nil {
		t.Fatalf("metadata saves=%d", client.saveCalls)
	}
	if got, err := os.ReadFile(client.committed.PathName); err != nil || string(got) != "oldnew" {
		t.Errorf("replay corrupted merged attachment: %q %v", got, err)
	}
}

func TestUploadSizeMismatchPreservesPartsBeforePublication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	previousClient := TaskServiceClient
	t.Cleanup(func() { TaskServiceClient = previousClient })
	client := &committedUploadClient{}
	TaskServiceClient = client
	fields := map[string]string{
		"projectCode": "project-code", "taskCode": "task-code", "filename": "size.txt",
		"identifier": "size-check", "totalChunks": "2", "chunkNumber": "1", "totalSize": "6",
	}
	if result := uploadChunkForTest(t, fields, "ab", 1, "organization-code"); result.Code != 200 {
		t.Fatalf("first chunk rejected: %+v", result)
	}
	fields["chunkNumber"] = "2"
	if result := uploadChunkForTest(t, fields, "cd", 1, "organization-code"); result.Code != 400 {
		t.Fatalf("size mismatch = %+v, want bad request", result)
	}
	dir := filepath.Join("upload", "project-code", "task-code", tms.FormatYMD(time.Now()))
	for i, want := range []string{"ab", "cd"} {
		part := uploadPartPath(dir, "size-check", i+1)
		if got, err := os.ReadFile(part); err != nil || string(got) != want {
			t.Errorf("size mismatch destroyed part %d: %q %v", i+1, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(uploadPartPath(dir, "size-check", 1)), "manifest.json")); err != nil {
		t.Errorf("size mismatch destroyed original contract: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "size.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("size mismatch left a published attachment: %v", err)
	}
	if client.saveCalls != 0 {
		t.Errorf("size mismatch created metadata %d times", client.saveCalls)
	}
	if leftovers, _ := filepath.Glob(filepath.Join(dir, ".assembling-*")); len(leftovers) != 0 {
		t.Errorf("failed merge leaked temporary files: %v", leftovers)
	}
}

func TestOutOfOrderUploadChecksAllStoredPartSizes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	previousClient := TaskServiceClient
	t.Cleanup(func() { TaskServiceClient = previousClient })
	for _, aggregate := range []bool{false, true} {
		t.Run(strconv.FormatBool(aggregate), func(t *testing.T) {
			client := &committedUploadClient{}
			TaskServiceClient = client
			fields := map[string]string{
				"projectCode": "project-code", "taskCode": "task-code", "filename": "quota-" + strconv.FormatBool(aggregate) + ".txt",
				"identifier": "quota-" + strconv.FormatBool(aggregate), "totalChunks": "2", "chunkNumber": "2", "totalSize": "6",
			}
			want := "oldnew"
			if aggregate {
				fields["totalChunks"], fields["chunkNumber"], fields["totalSize"] = "3", "3", "9"
				if r := uploadChunkForTest(t, fields, "last", 1, "organization-code"); r.Code != 200 {
					t.Fatalf("last-numbered part rejected: %+v", r)
				}
				fields["chunkNumber"] = "2"
				want = "himidlast"
			}
			if r := uploadChunkForTest(t, fields, "toolong", 1, "organization-code"); r.Code != 400 {
				t.Errorf("out-of-order oversized upload = %+v, want bad request", r)
			}
			dir := filepath.Join("upload", "project-code", "task-code", tms.FormatYMD(time.Now()))
			if _, err := os.Stat(uploadPartPath(dir, fields["identifier"], 2)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("rejected new part was retained: %v", err)
			}
			if aggregate {
				if got, err := os.ReadFile(uploadPartPath(dir, fields["identifier"], 3)); err != nil || string(got) != "last" {
					t.Fatalf("accepted other part changed: %q %v", got, err)
				}
			}
			if client.saveCalls != 0 {
				t.Errorf("incomplete upload saved metadata %d times", client.saveCalls)
			}
			parts := []string{"old", "new"}
			if aggregate {
				parts = []string{"hi", "mid"}
			}
			for i, part := range parts {
				fields["chunkNumber"] = strconv.Itoa(i + 1)
				if r := uploadChunkForTest(t, fields, part, 1, "organization-code"); r.Code != 200 {
					t.Fatalf("correct upload could not continue: %+v", r)
				}
			}
			if client.saveCalls != 1 || client.committed == nil {
				t.Fatalf("metadata saves=%d", client.saveCalls)
			}
			if got, err := os.ReadFile(client.committed.PathName); err != nil || string(got) != want {
				t.Fatalf("completed upload = %q %v, want %q", got, err, want)
			}
		})
	}
}

func TestUploadQuotaRejectionDoesNotDeleteExistingPart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	previousClient := TaskServiceClient
	t.Cleanup(func() { TaskServiceClient = previousClient })
	client := &committedUploadClient{}
	TaskServiceClient = client
	fields := map[string]string{
		"projectCode": "project-code", "taskCode": "task-code", "filename": "legacy-quota.txt",
		"identifier": "legacy-quota", "totalChunks": "3", "chunkNumber": "3", "totalSize": "9",
	}
	if r := uploadChunkForTest(t, fields, "old", 1, "organization-code"); r.Code != 200 {
		t.Fatal(r)
	}
	dir := filepath.Join("upload", "project-code", "task-code", tms.FormatYMD(time.Now()))
	// Model a set admitted by an older version that stopped counting at the
	// first missing part. Reject it without deleting the replayed original.
	if err := os.WriteFile(uploadPartPath(dir, "legacy-quota", 2), []byte("toolong"), 0600); err != nil {
		t.Fatal(err)
	}
	if r := uploadChunkForTest(t, fields, "old", 1, "organization-code"); r.Code != 400 {
		t.Errorf("old over-limit set replay = %+v, want bad request", r)
	}
	for number, want := range map[int]string{2: "toolong", 3: "old"} {
		if got, err := os.ReadFile(uploadPartPath(dir, "legacy-quota", number)); err != nil || string(got) != want {
			t.Errorf("quota rejection deleted saved part %d: %q %v", number, got, err)
		}
	}
	if client.saveCalls != 0 {
		t.Errorf("invalid upload saved metadata %d times", client.saveCalls)
	}
}

func TestOutOfOrderUploadEnforcesGlobalLimitWithoutDeclaredSize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	previousClient := TaskServiceClient
	t.Cleanup(func() { TaskServiceClient = previousClient })
	client := &committedUploadClient{}
	TaskServiceClient = client
	fields := map[string]string{
		"projectCode": "project-code", "taskCode": "task-code", "filename": "global-quota.txt",
		"identifier": "global-quota", "totalChunks": "3", "chunkNumber": "3", "totalSize": "0",
	}
	if r := uploadChunkForTest(t, fields, "old", 1, "organization-code"); r.Code != 200 {
		t.Fatal(r)
	}
	dir := filepath.Join("upload", "project-code", "task-code", tms.FormatYMD(time.Now()))
	last := uploadPartPath(dir, fields["identifier"], 3)
	if err := os.MkdirAll(filepath.Dir(last), 0700); err != nil {
		t.Fatal(err)
	}
	// A sparse file exercises the global bound without allocating a large body.
	f, err := os.Create(last)
	if err != nil {
		t.Fatal(err)
	}
	truncateErr, closeErr := f.Truncate(maxUploadSize), f.Close()
	if truncateErr != nil || closeErr != nil {
		t.Fatalf("prepare stored part: %v %v", truncateErr, closeErr)
	}
	fields["chunkNumber"] = "2"
	if r := uploadChunkForTest(t, fields, "new", 1, "organization-code"); r.Code != 400 {
		t.Fatalf("global limit bypassed with missing first part: %+v", r)
	}
	if _, err := os.Stat(uploadPartPath(dir, fields["identifier"], 2)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("rejected new part retained: %v", err)
	}
	if info, err := os.Stat(last); err != nil || info.Size() != maxUploadSize {
		t.Errorf("previously stored part changed: %v %v", info, err)
	}
	if client.saveCalls != 0 {
		t.Errorf("invalid upload saved metadata %d times", client.saveCalls)
	}
}

func TestStoreUploadPartPreservesAcceptedBytesOnReadFailure(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, failure := range []string{"read", "short"} {
			t.Run(strconv.FormatBool(existing)+"/"+failure, func(t *testing.T) {
				dir := t.TempDir()
				part := filepath.Join(dir, "1.part")
				if existing {
					if err := os.WriteFile(part, []byte("old"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				readError := errors.New("input failed after partial content")
				var input io.Reader = bytes.NewBufferString("ba")
				if failure == "read" {
					input = io.MultiReader(input, iotest.ErrReader(readError))
				}
				_, err := storeUploadPart(part, input, 3)
				if err == nil || (failure == "read" && !errors.Is(err, readError)) {
					t.Fatalf("storeUploadPart error=%v", err)
				}
				got, err := os.ReadFile(part)
				if existing {
					if err != nil || string(got) != "old" {
						t.Errorf("accepted content changed: %q %v", got, err)
					}
				} else if !errors.Is(err, os.ErrNotExist) {
					t.Errorf("failed new chunk was published: %q %v", got, err)
				}
				if leftovers, _ := filepath.Glob(filepath.Join(dir, ".incoming-*")); len(leftovers) != 0 {
					t.Errorf("failed write leaked temporary inputs: %v", leftovers)
				}
			})
		}
	}
}

func TestUploadIDRejectsChangedContractBeforeReplacingChunks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	previousClient := TaskServiceClient
	t.Cleanup(func() { TaskServiceClient = previousClient })
	for _, change := range []string{"filename", "totalChunks", "totalSize", "member", "organization"} {
		t.Run(change, func(t *testing.T) {
			client := &committedUploadClient{}
			TaskServiceClient = client
			fields := map[string]string{
				"projectCode": "project-code", "taskCode": "task-code", "filename": change + ".txt",
				"identifier": "contract-" + change, "totalChunks": "2", "chunkNumber": "1", "totalSize": "6",
			}
			if result := uploadChunkForTest(t, fields, "old", 1, "organization-code"); result.Code != 200 {
				t.Fatalf("first chunk rejected: %+v", result)
			}
			changed := make(map[string]string)
			for key, value := range fields {
				changed[key] = value
			}
			member, org := int64(1), "organization-code"
			switch change {
			case "filename":
				changed[change] = "other.txt"
			case "totalChunks":
				changed[change] = "3"
			case "totalSize":
				changed[change] = "8"
			case "member":
				member = 2
			case "organization":
				org = "other-organization"
			}
			if result := uploadChunkForTest(t, changed, "bad", member, org); result.Code != 409 {
				t.Errorf("changed upload contract = %+v, want conflict", result)
			}
			part := uploadPartPath(filepath.Join("upload", "project-code", "task-code", tms.FormatYMD(time.Now())), fields["identifier"], 1)
			if got, err := os.ReadFile(part); err != nil || string(got) != "old" {
				t.Errorf("conflicting request replaced old chunk: %q %v", got, err)
			}
			fields["chunkNumber"] = "2"
			if result := uploadChunkForTest(t, fields, "new", 1, "organization-code"); result.Code != 200 {
				t.Fatalf("original upload did not resume: %+v", result)
			}
			if client.saveCalls != 1 || client.committed == nil {
				t.Fatalf("metadata saves=%d", client.saveCalls)
			}
			if got, err := os.ReadFile(client.committed.PathName); err != nil || string(got) != "oldnew" {
				t.Errorf("resumed upload mixed content: %q %v", got, err)
			}
		})
	}
}

func TestUploadIDRejectsUnknownOriginalContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	previousClient := TaskServiceClient
	t.Cleanup(func() { TaskServiceClient = previousClient })
	for _, state := range []string{"missing", "corrupt"} {
		t.Run(state, func(t *testing.T) {
			client := &committedUploadClient{}
			TaskServiceClient = client
			fields := map[string]string{
				"projectCode": "project-code", "taskCode": "task-code", "filename": state + ".txt",
				"identifier": "unknown-" + state, "totalChunks": "2", "chunkNumber": "1", "totalSize": "6",
			}
			part := uploadPartPath(filepath.Join("upload", "project-code", "task-code", tms.FormatYMD(time.Now())), fields["identifier"], 1)
			if err := os.MkdirAll(filepath.Dir(part), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(part, []byte("old"), 0644); err != nil {
				t.Fatal(err)
			}
			if state == "corrupt" {
				if err := os.WriteFile(filepath.Join(filepath.Dir(part), "manifest.json"), []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if result := uploadChunkForTest(t, fields, "bad", 1, "organization-code"); result.Code == 200 {
				t.Errorf("unverified old chunks were reused: %+v", result)
			}
			if got, err := os.ReadFile(part); err != nil || string(got) != "old" || client.saveCalls != 0 {
				t.Fatalf("unknown original changed: content=%q err=%v metadata=%d", got, err, client.saveCalls)
			}
		})
	}
}

func TestUploadFilesCannotOverwriteExistingAttachment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDirectory) })
	originalClient := TaskServiceClient
	t.Cleanup(func() { TaskServiceClient = originalClient })
	for _, firstChunks := range []int{1, 2} {
		for _, nextChunks := range []int{1, 2} {
			t.Run(strconv.Itoa(firstChunks)+"-then-"+strconv.Itoa(nextChunks), func(t *testing.T) {
				client := &committedUploadClient{}
				TaskServiceClient = client
				filename := "same-name-" + strconv.Itoa(firstChunks) + "-" + strconv.Itoa(nextChunks) + ".txt"
				upload := func(content, identifier string, chunks int) common.Result {
					t.Helper()
					var result common.Result
					for number := 1; number <= chunks; number++ {
						var body bytes.Buffer
						writer := multipart.NewWriter(&body)
						for name, value := range map[string]string{
							"projectCode": "project-code", "taskCode": "task-code", "filename": filename,
							"totalChunks": strconv.Itoa(chunks), "chunkNumber": strconv.Itoa(number),
							"totalSize": strconv.Itoa(len(content)), "identifier": identifier,
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
						if chunks == 2 {
							payload = []string{content[:len(content)/2], content[len(content)/2:]}[number-1]
						}
						if _, err := file.Write([]byte(payload)); err != nil {
							t.Fatal(err)
						}
						if err := writer.Close(); err != nil {
							t.Fatal(err)
						}
						response := httptest.NewRecorder()
						c, _ := gin.CreateTestContext(response)
						c.Request = httptest.NewRequest(http.MethodPost, "/upload", &body)
						c.Request.Header.Set("Content-Type", writer.FormDataContentType())
						c.Set("memberId", int64(1))
						c.Set("organizationCode", "organization-code")
						(&HandlerTask{}).uploadFiles(c)
						if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
							t.Fatal(err)
						}
					}
					return result
				}
				if result := upload("original attachment", "first-"+filename, firstChunks); result.Code != 200 {
					t.Fatalf("initial upload failed: %+v", result)
				}
				originalPath := client.committed.PathName
				if result := upload("different replacement", "second-"+filename, nextChunks); result.Code != 409 {
					t.Errorf("same-name upload = %+v, want conflict", result)
				}
				if got, err := os.ReadFile(filepath.FromSlash(originalPath)); err != nil || string(got) != "original attachment" {
					t.Errorf("existing attachment overwritten: content=%q err=%v", got, err)
				}
				if client.saveCalls != 1 {
					t.Errorf("conflicting upload created metadata: calls=%d", client.saveCalls)
				}
				if nextChunks == 2 {
					for number := 1; number <= 2; number++ {
						part := uploadPartPath(filepath.Dir(originalPath), "second-"+filename, number)
						if _, err := os.Stat(part); err != nil {
							t.Errorf("conflicting merge lost retry input: %v", err)
						}
					}
				}
			})
		}
	}
}

func TestUploadChunkCannotOverwritePublishedAttachment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	client := TaskServiceClient
	TaskServiceClient = &committedUploadClient{}
	t.Cleanup(func() { TaskServiceClient = client })
	directory := filepath.Join("upload", "project-code", "task-code", tms.FormatYMD(time.Now()))
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(directory, "collision.1.part")
	if err := os.WriteFile(oldPath, []byte("published attachment"), 0644); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range map[string]string{
		"projectCode": "project-code", "taskCode": "task-code", "filename": "fresh.txt",
		"identifier": "collision", "totalChunks": "2", "chunkNumber": "1", "totalSize": "6",
	} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	file, err := writer.CreateFormFile("file", "fresh.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/upload", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Set("memberId", int64(1))
	c.Set("organizationCode", "organization-code")
	(&HandlerTask{}).uploadFiles(c)
	var result common.Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Code != 200 {
		t.Fatalf("first chunk failed: %+v, %v", result, err)
	}
	if got, err := os.ReadFile(oldPath); err != nil || string(got) != "published attachment" {
		t.Fatalf("chunk path collided with published attachment: content=%q err=%v", got, err)
	}
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
	if err := os.MkdirAll(filepath.Dir(uploadPartPath(directory, identifier, 1)), 0755); err != nil {
		t.Fatal(err)
	}
	parts := map[int]string{1: "first\n", 2: "second\n", 3: "third\n"}
	for number, content := range parts {
		path := uploadPartPath(directory, identifier, number)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	finalPath, err := mergeUploadParts(directory, identifier, filename, 3, 0)
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
		partPath := uploadPartPath(directory, identifier, number)
		if _, err := os.Stat(partPath); !os.IsNotExist(err) {
			t.Fatalf("part %s was not cleaned up: %v", partPath, err)
		}
	}
}

func TestMergeUploadPartsCleansTemporaryFileWhenPartIsMissing(t *testing.T) {
	directory := t.TempDir()
	identifier := "upload-id"
	filename := "report.txt"
	if err := os.MkdirAll(filepath.Dir(uploadPartPath(directory, identifier, 1)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(uploadPartPath(directory, identifier, 1), []byte("first"), 0644); err != nil {
		t.Fatal(err)
	}

	finalPath, err := mergeUploadParts(directory, identifier, filename, 2, 0)
	if err == nil {
		t.Fatal("mergeUploadParts succeeded with a missing part")
	}
	if finalPath != "" {
		t.Fatalf("final path = %q, want empty path on failure", finalPath)
	}
	if _, statErr := os.Stat(filepath.Join(directory, filename+".assembling")); !os.IsNotExist(statErr) {
		t.Fatalf("temporary assembling file was not removed: %v", statErr)
	}
	if content, readErr := os.ReadFile(uploadPartPath(directory, identifier, 1)); readErr != nil || string(content) != "first" {
		t.Fatalf("failed merge must preserve the existing part for retry: content=%q, err=%v", content, readErr)
	}
}

func TestMergeUploadPartsPreservesPartsWhenPublishFails(t *testing.T) {
	directory := t.TempDir()
	identifier := "upload-id"
	filename := "report.txt"
	if err := os.MkdirAll(filepath.Dir(uploadPartPath(directory, identifier, 1)), 0755); err != nil {
		t.Fatal(err)
	}
	for number, content := range []string{"first", "second"} {
		part := uploadPartPath(directory, identifier, number+1)
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
	if finalPath, err := mergeUploadParts(directory, identifier, filename, 2, 0); err == nil || finalPath != "" {
		t.Fatalf("merge with a blocked destination = (%q, %v), want a failure", finalPath, err)
	}
	for number, want := range []string{"first", "second"} {
		part := uploadPartPath(directory, identifier, number+1)
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
	finalPath, err := mergeUploadParts(directory, identifier, filename, 2, 0)
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
