//go:build linux || darwin

package configeditor

import (
	"os"
	"path/filepath"
	"testing"
	"golang.org/x/sys/unix"
)

func TestRejectFIFOWithoutBlocking(t *testing.T) {
	p:=filepath.Join(t.TempDir(),"pipe")
	if err:=unix.Mkfifo(p,0600);err!=nil{t.Fatal(err)}
	if _,_,err:=(&Store{Path:p}).Load();err==nil{t.Fatal("FIFO accepted")}
}

func TestRejectSymlinkAtOpen(t *testing.T) {
	d:=t.TempDir();p:=filepath.Join(d,"config");target:=filepath.Join(d,"target")
	if err:=os.WriteFile(target,[]byte("station-callsign: N0CALL\n"),0600);err!=nil{t.Fatal(err)}
	if err:=os.Symlink(target,p);err!=nil{t.Fatal(err)}
	if _,_,err:=(&Store{Path:p}).Load();err==nil{t.Fatal("symlink accepted")}
}
