/*
Copyright Hitachi America, Ltd.

SPDX-License-Identifier: Apache-2.0
*/

package node

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/hyperledger/fabric/common/viperutil"
	"github.com/hyperledger/fabric/core/handlers/library"
	"github.com/hyperledger/fabric/core/peer"
	"github.com/hyperledger/fabric/core/testutil"
	msptesttools "github.com/hyperledger/fabric/msp/mgmt/testtools"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestStartCmd(t *testing.T) {
	defer viper.Reset()
	g := NewGomegaWithT(t)

	tempDir, err := ioutil.TempDir("", "startcmd")
	g.Expect(err).NotTo(HaveOccurred())
	defer os.RemoveAll(tempDir)

	viper.Set("peer.address", "localhost:6051")
	viper.Set("peer.listenAddress", "0.0.0.0:6051")
	viper.Set("peer.chaincodeListenAddress", "0.0.0.0:6052")
	viper.Set("peer.fileSystemPath", tempDir)
	viper.Set("chaincode.executetimeout", "30s")
	viper.Set("chaincode.mode", "dev")
	viper.Set("vm.endpoint", "unix:///var/run/docker.sock")

	msptesttools.LoadMSPSetupForTesting()

	go func() {
		cmd := startCmd()
		assert.NoError(t, cmd.Execute(), "expected to successfully start command")
	}()

	grpcProbe := func(addr string) bool {
		c, err := grpc.Dial(addr, grpc.WithBlock(), grpc.WithInsecure())
		if err == nil {
			c.Close()
			return true
		}
		return false
	}
	g.Eventually(grpcProbe("localhost:6051")).Should(BeTrue())
}

func TestAdminHasSeparateListener(t *testing.T) {
	assert.False(t, adminHasSeparateListener("0.0.0.0:7051", ""))

	assert.Panics(t, func() {
		adminHasSeparateListener("foo", "blabla")
	})

	assert.Panics(t, func() {
		adminHasSeparateListener("0.0.0.0:7051", "blabla")
	})

	assert.False(t, adminHasSeparateListener("0.0.0.0:7051", "0.0.0.0:7051"))
	assert.False(t, adminHasSeparateListener("0.0.0.0:7051", "127.0.0.1:7051"))
	assert.True(t, adminHasSeparateListener("0.0.0.0:7051", "0.0.0.0:7055"))
}

func TestHandlerMap(t *testing.T) {
	config1 := `
  peer:
    handlers:
      authFilters:
        -
          name: filter1
          library: /opt/lib/filter1.so
        -
          name: filter2
  `
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(bytes.NewBuffer([]byte(config1)))
	assert.NoError(t, err)

	libConf := library.Config{}
	err = viperutil.EnhancedExactUnmarshalKey("peer.handlers", &libConf)
	assert.NoError(t, err)
	assert.Len(t, libConf.AuthFilters, 2, "expected two filters")
	assert.Equal(t, "/opt/lib/filter1.so", libConf.AuthFilters[0].Library)
	assert.Equal(t, "filter2", libConf.AuthFilters[1].Name)
}

func TestComputeChaincodeEndpoint(t *testing.T) {
	/*** Scenario 1: chaincodeAddress and chaincodeListenAddress are not set ***/
	coreConfig := &peer.Config{}
	// Scenario 1.1: peer address is 0.0.0.0
	// computeChaincodeEndpoint will return error
	peerAddress0 := "0.0.0.0"
	ccEndpoint, err := computeChaincodeEndpoint(coreConfig, peerAddress0)
	assert.Error(t, err)
	assert.Equal(t, "", ccEndpoint)
	// Scenario 1.2: peer address is not 0.0.0.0
	// chaincodeEndpoint will be peerAddress:7052
	peerAddress := "127.0.0.1"
	ccEndpoint, err = computeChaincodeEndpoint(coreConfig, peerAddress)
	assert.NoError(t, err)
	assert.Equal(t, peerAddress+":7052", ccEndpoint)

	/*** Scenario 2: set up chaincodeListenAddress only ***/
	// Scenario 2.1: chaincodeListenAddress is 0.0.0.0
	chaincodeListenPort := "8052"
	settingChaincodeListenAddress0 := "0.0.0.0:" + chaincodeListenPort
	coreConfig.ChaincodeListenAddress = settingChaincodeListenAddress0
	coreConfig.ChaincodeAddress = ""
	// Scenario 2.1.1: peer address is 0.0.0.0
	// computeChaincodeEndpoint will return error
	ccEndpoint, err = computeChaincodeEndpoint(coreConfig, peerAddress0)
	assert.Error(t, err)
	assert.Equal(t, "", ccEndpoint)
	// Scenario 2.1.2: peer address is not 0.0.0.0
	// chaincodeEndpoint will be peerAddress:chaincodeListenPort
	ccEndpoint, err = computeChaincodeEndpoint(coreConfig, peerAddress)
	assert.NoError(t, err)
	assert.Equal(t, peerAddress+":"+chaincodeListenPort, ccEndpoint)
	// Scenario 2.2: chaincodeListenAddress is not 0.0.0.0
	// chaincodeEndpoint will be chaincodeListenAddress
	settingChaincodeListenAddress := "127.0.0.1:" + chaincodeListenPort
	coreConfig.ChaincodeListenAddress = settingChaincodeListenAddress
	coreConfig.ChaincodeAddress = ""
	ccEndpoint, err = computeChaincodeEndpoint(coreConfig, peerAddress)
	assert.NoError(t, err)
	assert.Equal(t, settingChaincodeListenAddress, ccEndpoint)
	// Scenario 2.3: chaincodeListenAddress is invalid
	// computeChaincodeEndpoint will return error
	settingChaincodeListenAddressInvalid := "abc"
	coreConfig.ChaincodeListenAddress = settingChaincodeListenAddressInvalid
	coreConfig.ChaincodeAddress = ""
	ccEndpoint, err = computeChaincodeEndpoint(coreConfig, peerAddress)

	assert.Error(t, err)
	assert.Equal(t, "", ccEndpoint)

	/*** Scenario 3: set up chaincodeAddress only ***/
	// Scenario 3.1: chaincodeAddress is 0.0.0.0
	// computeChaincodeEndpoint will return error
	chaincodeAddressPort := "9052"
	settingChaincodeAddress0 := "0.0.0.0:" + chaincodeAddressPort
	coreConfig.ChaincodeListenAddress = ""
	coreConfig.ChaincodeAddress = settingChaincodeAddress0
	ccEndpoint, err = computeChaincodeEndpoint(coreConfig, peerAddress)

	assert.Error(t, err)
	assert.Equal(t, "", ccEndpoint)
	// Scenario 3.2: chaincodeAddress is not 0.0.0.0
	// chaincodeEndpoint will be chaincodeAddress
	settingChaincodeAddress := "127.0.0.2:" + chaincodeAddressPort
	coreConfig.ChaincodeListenAddress = ""
	coreConfig.ChaincodeAddress = settingChaincodeAddress
	ccEndpoint, err = computeChaincodeEndpoint(coreConfig, peerAddress)
	assert.NoError(t, err)
	assert.Equal(t, settingChaincodeAddress, ccEndpoint)
	// Scenario 3.3: chaincodeAddress is invalid
	// computeChaincodeEndpoint will return error
	settingChaincodeAddressInvalid := "bcd"
	coreConfig.ChaincodeListenAddress = ""
	coreConfig.ChaincodeAddress = settingChaincodeAddressInvalid
	ccEndpoint, err = computeChaincodeEndpoint(coreConfig, peerAddress)
	assert.Error(t, err)
	assert.Equal(t, "", ccEndpoint)

	/*** Scenario 4: set up both chaincodeAddress and chaincodeListenAddress ***/
	// This scenario will be the same to scenarios 3: set up chaincodeAddress only.
}

func TestGetDockerHostConfig(t *testing.T) {
	testutil.SetupTestConfig()
	hostConfig := getDockerHostConfig()
	assert.NotNil(t, hostConfig)
	assert.Equal(t, "host", hostConfig.NetworkMode)
	assert.Equal(t, "json-file", hostConfig.LogConfig.Type)
	assert.Equal(t, "50m", hostConfig.LogConfig.Config["max-size"])
	assert.Equal(t, "5", hostConfig.LogConfig.Config["max-file"])
	assert.Equal(t, int64(1024*1024*1024*2), hostConfig.Memory)
	assert.Equal(t, int64(0), hostConfig.CPUShares)
}