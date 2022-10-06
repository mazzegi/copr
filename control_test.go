package copr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mazzegi/copr/coprtest"
	"github.com/mazzegi/log"
	"github.com/pkg/errors"
)

func copyFile(dst string, src string) error {
	srcF, err := os.Open(src)
	if err != nil {
		return errors.Wrapf(err, "open source %q", src)
	}
	defer srcF.Close()

	dstF, err := os.Create(dst)
	if err != nil {
		return errors.Wrapf(err, "create dest %q", dst)
	}
	defer dstF.Close()

	_, err = io.Copy(dstF, srcF)
	if err != nil {
		return errors.Wrapf(err, "copy %q to %q", src, dst)
	}
	return nil
}

func bootstrapTestDeployment(dir string, unitNum int, env []string, enabled bool) error {
	prgName := "test_unit"
	err := buildPrg(dir, prgName)
	if err != nil {
		return errors.Wrapf(err, "build-prg %q in %q", prgName, dir)
	}
	// create unit file
	unitConf := UnitConfig{
		Enabled:         enabled,
		Program:         prgName,
		Args:            []string{fmt.Sprintf("-bind=127.0.0.1:%d", 31000+unitNum)},
		Env:             env,
		RestartAfterSec: 1,
	}
	unitFilePath := filepath.Join(dir, "copr.unit.json")
	unitF, err := os.Create(unitFilePath)
	if err != nil {
		return errors.Wrapf(err, "create-file %q", unitFilePath)
	}
	defer unitF.Close()
	enc := json.NewEncoder(unitF)
	enc.SetIndent("", "  ")
	err = enc.Encode(unitConf)
	if err != nil {
		return errors.Wrapf(err, "json-encode unit-conf for %q", unitName(unitNum))
	}
	return nil
}

func bootstrapTestUnits(dir string, unitCount int, env []string) error {

	prgName := "test_unit"
	err := buildPrg(dir, prgName)
	if err != nil {
		return errors.Wrapf(err, "build-prg %q in %q", prgName, dir)
	}
	prgPath := filepath.Join(dir, prgName)

	for i := 1; i <= unitCount; i++ {
		unitName := fmt.Sprintf("unit_%02d", i)
		unitDir := filepath.Join(dir, unitName)
		err = os.Mkdir(unitDir, os.ModePerm)
		if err != nil {
			return errors.Wrapf(err, "mkdir %q", unitDir)
		}
		//copy prg to unit dir
		dstPrgPath := filepath.Join(unitDir, prgName)
		err = copyFile(dstPrgPath, prgPath)
		if err != nil {
			return errors.Wrap(err, "copy program")
		}
		err = os.Chmod(dstPrgPath, 0755)
		if err != nil {
			return errors.Wrapf(err, "chmod +x on %q", dstPrgPath)
		}

		// create unit file
		unitConf := UnitConfig{
			Enabled:         true,
			Program:         prgName,
			Args:            []string{fmt.Sprintf("-bind=127.0.0.1:%d", 31000+i)},
			Env:             env,
			RestartAfterSec: 1,
		}
		unitFilePath := filepath.Join(unitDir, "copr.unit.json")
		unitF, err := os.Create(unitFilePath)
		if err != nil {
			return errors.Wrapf(err, "create-file %q", unitFilePath)
		}
		defer unitF.Close()
		enc := json.NewEncoder(unitF)
		enc.SetIndent("", "  ")
		err = enc.Encode(unitConf)
		if err != nil {
			return errors.Wrapf(err, "json-encode unit-conf for %q", unitName)
		}
	}

	return nil
}

func assertUnitRunning(t *testing.T, unitNum int) {
	url := fmt.Sprintf("http://127.0.0.1:%d", 31000+unitNum)
	err := sendRequest(url, coprtest.TestCommand{Action: coprtest.TestActionProbe})
	assertNoErr(t, err, "assert-running unit no %d", unitNum)
	log.Debugf("probe %d on %q - OK", unitNum, url)
}

func assertUnitNotRunning(t *testing.T, unitNum int) {
	url := fmt.Sprintf("http://127.0.0.1:%d", 31000+unitNum)
	err := sendRequest(url, coprtest.TestCommand{Action: coprtest.TestActionProbe})
	assertErr(t, err, "assert-not-running unit no %d", unitNum)
	log.Debugf("probe %d on %q - FAILED (which is OK)", unitNum, url)
}

func unitName(unitNum int) string {
	return fmt.Sprintf("unit_%02d", unitNum)
}

func TestController(t *testing.T) {
	unitsDir := "tmp_test_controller"
	err := os.MkdirAll(unitsDir, os.ModePerm)
	assertNoErr(t, err, "mkdirall %q", unitsDir)
	defer os.RemoveAll(unitsDir)

	unitCount := 5
	err = bootstrapTestUnits(unitsDir, unitCount, []string{})
	assertNoErr(t, err, "bootstrap in %q", unitsDir)

	//secrets
	secFile := filepath.Join(unitsDir, "copr.secrets")
	sec, err := NewSecrets(secFile, "controller-test-pwd")
	assertNoErr(t, err, "new-secrets in %q", secFile)

	sec.Set("foo", "bar")
	sec.Set("baz", "acme")

	ctrl, err := NewController(unitsDir, sec)
	assertNoErr(t, err, "new-controller")

	ctx, cancel := context.WithCancel(context.Background())

	ctrlDoneC := make(chan struct{})
	go func() {
		defer close(ctrlDoneC)
		ctrl.RunCtx(ctx)
	}()

	checkStatusAfter := 50 * time.Millisecond
	assertAllRunning := func() {
		for i := 1; i <= unitCount; i++ {
			assertUnitRunning(t, i)
		}
	}
	assertNoneRunning := func() {
		for i := 1; i <= unitCount; i++ {
			assertUnitNotRunning(t, i)
		}
	}

	// start tests
	assertNoErr(t, ctrl.StartAll().Error(), "start-all")
	<-time.After(checkStatusAfter)
	assertAllRunning()

	//stop first
	assertNoErr(t, ctrl.Stop(unitName(1)).Error(), "stop first")
	<-time.After(checkStatusAfter)
	assertUnitNotRunning(t, 1)

	//stop last
	assertNoErr(t, ctrl.Stop(unitName(unitCount)).Error(), "stop last")
	<-time.After(checkStatusAfter)
	assertUnitNotRunning(t, unitCount)

	//stop one in the middle
	ucm := (unitCount + 1) / 2
	assertNoErr(t, ctrl.Stop(unitName(ucm)).Error(), "stop one in the middle")
	<-time.After(checkStatusAfter)
	assertUnitNotRunning(t, ucm)

	// starting them again
	assertNoErr(t, ctrl.Start(unitName(1)).Error(), "start first")
	assertNoErr(t, ctrl.Start(unitName(ucm)).Error(), "start one in the middle")
	assertNoErr(t, ctrl.Start(unitName(unitCount)).Error(), "start last")
	<-time.After(checkStatusAfter)
	assertAllRunning()

	// stop all
	assertNoErr(t, ctrl.StopAll().Error(), "stop all")
	<-time.After(checkStatusAfter)
	assertNoneRunning()

	// start all again
	assertNoErr(t, ctrl.StartAll().Error(), "stop all")
	<-time.After(checkStatusAfter)
	assertAllRunning()

	//disable first
	assertNoErr(t, ctrl.Disable(unitName(1)).Error(), "disable first")
	<-time.After(checkStatusAfter)
	assertUnitNotRunning(t, 1)
	//enable
	assertNoErr(t, ctrl.Enable(unitName(1)).Error(), "enable first")
	<-time.After(checkStatusAfter)
	assertUnitNotRunning(t, 1)
	// ... and start again
	assertNoErr(t, ctrl.Start(unitName(1)).Error(), "start first")
	<-time.After(checkStatusAfter)
	assertUnitRunning(t, 1)

	//finish
	<-time.After(50 * time.Millisecond)
	cancel()
	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("controller didn't finish after 5 secs")
	case <-ctrlDoneC:
		log.Debugf("controller gracefully finished")
	}

}

func TestControllerDeploy(t *testing.T) {
	tmpDir := "tmp"
	unitsDir := filepath.Join(tmpDir, "tmp_test_controller")
	err := os.MkdirAll(unitsDir, os.ModePerm)
	assertNoErr(t, err, "mkdirall %q", unitsDir)
	defer os.RemoveAll(tmpDir)

	unitCount := 2
	err = bootstrapTestUnits(unitsDir, unitCount, []string{})
	assertNoErr(t, err, "bootstrap in %q", unitsDir)

	//deployment
	deploymentCreateDir := filepath.Join(tmpDir, "deployment_create")
	err = os.MkdirAll(deploymentCreateDir, os.ModePerm)
	assertNoErr(t, err, "mkdirall %q", deploymentCreateDir)
	err = bootstrapTestDeployment(deploymentCreateDir, 3, []string{}, true)
	assertNoErr(t, err, "bootstrap deployment in %q", deploymentCreateDir)

	deploymentUpdateDir := filepath.Join(tmpDir, "deployment_update")
	err = os.MkdirAll(deploymentUpdateDir, os.ModePerm)
	assertNoErr(t, err, "mkdirall %q", deploymentUpdateDir)
	err = bootstrapTestDeployment(deploymentUpdateDir, 1, []string{}, false)
	assertNoErr(t, err, "bootstrap deployment in %q", deploymentUpdateDir)

	//secrets
	secFile := filepath.Join(unitsDir, "copr.secrets")
	sec, err := NewSecrets(secFile, "controller-test-pwd")
	assertNoErr(t, err, "new-secrets in %q", secFile)

	sec.Set("foo", "bar")
	sec.Set("baz", "acme")

	ctrl, err := NewController(unitsDir, sec)
	assertNoErr(t, err, "new-controller")

	ctx, cancel := context.WithCancel(context.Background())

	ctrlDoneC := make(chan struct{})
	go func() {
		defer close(ctrlDoneC)
		ctrl.RunCtx(ctx)
	}()

	checkStatusAfter := 50 * time.Millisecond
	assertAllRunning := func() {
		for i := 1; i <= unitCount; i++ {
			assertUnitRunning(t, i)
		}
	}
	assertNoneRunning := func() {
		for i := 1; i <= unitCount; i++ {
			assertUnitNotRunning(t, i)
		}
	}

	// start tests
	assertNoneRunning()
	assertNoErr(t, ctrl.StartAll().Error(), "start-all")
	<-time.After(checkStatusAfter)
	assertAllRunning()

	//deploy new
	err = ctrl.Deploy(unitName(unitCount+1), deploymentCreateDir).Error()
	assertNoErr(t, err, "deploy-create")
	unitCount++
	<-time.After(checkStatusAfter)
	assertAllRunning()

	// deploy existing - disabled
	err = ctrl.Deploy(unitName(1), deploymentUpdateDir).Error()
	assertNoErr(t, err, "deploy-update")
	<-time.After(checkStatusAfter)
	assertUnitNotRunning(t, 1)

	//finish
	<-time.After(50 * time.Millisecond)
	cancel()
	select {
	case <-time.After(5 * time.Second):
		t.Fatalf("controller didn't finish after 5 secs")
	case <-ctrlDoneC:
		log.Debugf("controller gracefully finished")
	}

}
