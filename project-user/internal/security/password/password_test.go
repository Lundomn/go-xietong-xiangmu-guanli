package password

import "testing"

func TestHashAndVerify(t *testing.T) {
	hash, err := Hash("正确的密码-123")
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}
	if ok, legacy := Verify(hash, "正确的密码-123"); !ok || legacy {
		t.Fatalf("bcrypt password did not verify correctly: ok=%v legacy=%v", ok, legacy)
	}
	if ok, _ := Verify(hash, "错误的密码"); ok {
		t.Fatal("wrong password verified")
	}
}

func TestLegacyMD5PasswordCanMigrate(t *testing.T) {
	legacy := "098f6bcd4621d373cade4e832627b4f6" // md5("test")
	if ok, legacyFormat := Verify(legacy, "test"); !ok || !legacyFormat {
		t.Fatalf("legacy MD5 password was not detected: ok=%v legacy=%v", ok, legacyFormat)
	}
}
