package ovftool

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/mitchellh/packer/common"
	"github.com/mitchellh/packer/packer"
	"os/exec"
	"strconv"
	"strings"
)

var executable string = "ovftool"

type Config struct {
	common.PackerConfig `mapstructure:",squash"`
	TargetPath          string `mapstructure:"target"`
	TargetType          string `mapstructure:"format"`
	Compression         uint   `mapstructure:"compression"`
	tpl                 *packer.ConfigTemplate
}

type OVFPostProcessor struct {
	cfg Config
}

type OutputPathTemplate struct {
	ArtifactId string
	BuildName  string
	Provider   string
}

func (p *OVFPostProcessor) Configure(raws ...interface{}) error {
	_, err := common.DecodeConfig(&p.cfg, raws...)
	if err != nil {
		return err
	}
	p.cfg.tpl, err = packer.NewConfigTemplate()
	if err != nil {
		return err
	}
	p.cfg.tpl.UserVars = p.cfg.PackerUserVars

	if p.cfg.TargetType == "" {
		p.cfg.TargetType = "ovf"
	}

	if p.cfg.TargetPath == "" {
		p.cfg.TargetPath = "packer_{{ .BuildName }}_{{.Provider}}"
		if p.cfg.TargetType == "ova" {
			p.cfg.TargetPath += ".ova"
		}
	}

	errs := new(packer.MultiError)

	_, err = exec.LookPath(executable)
	if err != nil {
		errs = packer.MultiErrorAppend(
			errs, fmt.Errorf("Error: Could not find ovftool executable.", err))
	}

	if err = p.cfg.tpl.Validate(p.cfg.TargetPath); err != nil {
		errs = packer.MultiErrorAppend(
			errs, fmt.Errorf("Error parsing target template: %s", err))
	}

	if !(p.cfg.TargetType == "ovf" || p.cfg.TargetType == "ova") {
		errs = packer.MultiErrorAppend(
			errs, errors.New("Invalid target type. Only 'ovf' or 'ova' are allowed."))
	}

	if !(p.cfg.Compression >= 0 && p.cfg.Compression <= 9) {
		errs = packer.MultiErrorAppend(
			errs, errors.New("Invalid compression level. Must be between 1 and 9, or 0 for no compression."))
	}

	if len(errs.Errors) > 0 {
		return errs
	}

	return nil
}

func (p *OVFPostProcessor) PostProcess(ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, error) {
	if artifact.BuilderId() != "mitchellh.vmware" {
		return nil, false, fmt.Errorf("ovftool post-processor can only be used on VMware boxes: %s", artifact.BuilderId())
	}

	vmx := ""
	for _, path := range artifact.Files() {
		if strings.HasSuffix(path, ".vmx") {
			vmx = path
		}
	}
	if vmx == "" {
		return nil, false, fmt.Errorf("VMX file could not be located.")
	}

	targetPath, err := p.cfg.tpl.Process(p.cfg.TargetPath, &OutputPathTemplate{
		ArtifactId: artifact.Id(),
		BuildName:  p.cfg.PackerBuildName,
		Provider:   "vmware",
	})
	if err != nil {
		return nil, false, err
	}

	compression := ""
	if p.cfg.Compression > 0 {
		compression = "--compress=" + strconv.Itoa(int(p.cfg.Compression))
	}

	cmd := exec.Command(
		executable,
		"--targetType="+p.cfg.TargetType,
		"--acceptAllEulas",
		compression,
		vmx,
		targetPath,
	)
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	cmd.Stderr = &buffer
	err = cmd.Run()
	if err != nil {
		return nil, false, fmt.Errorf("Unable to execute ovftool. ", buffer.String())
	}
	ui.Message(fmt.Sprintf("%s", buffer.String()))

	return artifact, false, nil
}
