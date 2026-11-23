package configeditor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavePreservesBytesAndDetectsConflict(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yml")
	original := []byte("# existing comment\\nstation-callsign: N0CALL\\n")
	original = []byte(strings.ReplaceAll(string(original), "\\n", "\n"))
	if err := os.WriteFile(p, original, 0600); err != nil { t.Fatal(err) }
	s := &Store{Path:p}
	got, rev, err := s.Load()
	if err != nil || string(got)!=string(original) { t.Fatalf("load: %q, %v",got,err) }
	next := []byte("# keep my comment\nstation-callsign: N0CALL-10\n")
	newRev, err := s.Save(next,rev)
	if err != nil || newRev==rev { t.Fatalf("save: %v",err) }
	stored,err:=os.ReadFile(p)
	if err!=nil || string(stored)!=string(next) { t.Fatalf("stored: %q, %v",stored,err) }
	if _,err:=s.Save(original,rev); !errors.Is(err,ErrConflict) { t.Fatalf("expected conflict: %v",err) }
}

func TestInvalidAndExternalEditsAreRejected(t *testing.T) {
	p:=filepath.Join(t.TempDir(),"config.yml")
	if err:=os.WriteFile(p,[]byte("station-callsign: N0CALL\n"),0600); err!=nil {t.Fatal(err)}
	s:=&Store{Path:p}
	_,rev,err:=s.Load();if err!=nil {t.Fatal(err)}
	if _,err:=s.Save([]byte("transmitter:\n  audio-level: 4\n"),rev);err==nil {t.Fatal("accepted invalid audio level")}
	if err:=os.WriteFile(p,[]byte("station-callsign: EXTERNAL\n"),0600);err!=nil {t.Fatal(err)}
	if _,err:=s.Save([]byte("station-callsign: N0CALL\n"),rev);!errors.Is(err,ErrConflict) {t.Fatalf("expected conflict: %v",err)}
}

func TestRejectSymlinkAndOversizedFile(t *testing.T) {
	d:=t.TempDir();target:=filepath.Join(d,"target");link:=filepath.Join(d,"link")
	if err:=os.WriteFile(target,[]byte("station-callsign: N0CALL\n"),0600);err!=nil{t.Fatal(err)}
	if err:=os.Symlink(target,link);err!=nil{t.Fatal(err)}
	if _,_,err:=(&Store{Path:link}).Load();err==nil{t.Fatal("symlink accepted")}
	if err:=os.WriteFile(target,[]byte(strings.Repeat("x",MaxDocumentSize+1)),0600);err!=nil{t.Fatal(err)}
	if _,_,err:=(&Store{Path:target}).Load();err==nil{t.Fatal("oversized file accepted")}
}
