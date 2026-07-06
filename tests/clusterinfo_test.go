package kcontext_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gqlo/kcontext"
)

func TestFetchClusterMeta_fromFakeOc(t *testing.T) {
	dir := t.TempDir()
	ocPath := filepath.Join(dir, "oc")
	script := `#!/bin/sh
case "$*" in
  *get\ nodes\ --no-headers*)
    echo "node-a Ready worker"
    echo "node-b Ready worker"
    echo "node-c Ready control-plane,master"
    ;;
  *clusterversion*version*)
    echo -n "4.16.10"
    ;;
  *openshift-cnv*kubevirt-hyperconverged*)
    echo -n "4.16.4"
    ;;
  *openshift-storage*odf-operator*)
    echo -n "4.16.0-rhodf"
    ;;
  *)
    echo "unexpected oc $*" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(ocPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	meta := kcontext.FetchClusterMeta(context.Background())
	if meta.MasterNodes != "1" {
		t.Errorf("MasterNodes = %q, want 1", meta.MasterNodes)
	}
	if meta.WorkerNodes != "2" {
		t.Errorf("WorkerNodes = %q, want 2", meta.WorkerNodes)
	}
	if meta.NodesDisplay() != "1 master · 2 worker" {
		t.Errorf("NodesDisplay() = %q, want %q", meta.NodesDisplay(), "1 master · 2 worker")
	}
	if meta.OCPVersion != "4.16.10" {
		t.Errorf("OCPVersion = %q, want 4.16.10", meta.OCPVersion)
	}
	if meta.CNVVersion != "4.16.4" {
		t.Errorf("CNVVersion = %q, want 4.16.4", meta.CNVVersion)
	}
	if meta.ODFVersion != "4.16.0-rhodf" {
		t.Errorf("ODFVersion = %q, want 4.16.0-rhodf", meta.ODFVersion)
	}
}
