package framework

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"
)

type ManifestTemplate struct {
	Image string
}

func (f *Framework) ApplyManifest() error {
	data, err := f.readManifest()
	if err != nil {
		return err
	}

	return ApplyManifest("apply", data)

}

func (f *Framework) DeleteManifest() error {
	data, err := f.readManifest()
	if err != nil {
		return err
	}

	return ApplyManifest("delete", data)
}

func (f *Framework) readManifest() (string, error)  {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	data, err := ioutil.ReadFile(filepath.Join(dir, "manifests/ccm-linode.yaml"))
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("csidriver").Parse(string(data))
	if err != nil {
		return "", err
	}
	var tmplBuf bytes.Buffer
	err = tmpl.Execute(&tmplBuf, ManifestTemplate{
		Image: Image,
	})
	if err != nil {
		return "", err
	}
	return tmplBuf.String(), nil
}

func (f *Framework) WaitForReadyDriver() error  {
	return nil
	/*return wait.Poll(retryInterval, retryTimout, func() (bool, error) {
		f.kubeClient.AppsV1().DaemonSets(core.NamespaceSystem).Get("csi-linode-node", metav1.GetOptions{})
	} )
*/
}

