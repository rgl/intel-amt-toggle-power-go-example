package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/device-management-toolkit/go-wsman-messages/v2/pkg/wsman"
	"github.com/device-management-toolkit/go-wsman-messages/v2/pkg/wsman/cim/models"
	"github.com/device-management-toolkit/go-wsman-messages/v2/pkg/wsman/cim/power"
	"github.com/device-management-toolkit/go-wsman-messages/v2/pkg/wsman/client"
)

// see https://github.com/intel/lms/blob/f7c374745ae7efb3ed7860fdc3f8abbb52dc9f8f/CIM_Framework/CIMFramework/CPPClasses/Include/CIM_AssociatedPowerManagementService.h#L143-L159
// see https://schemas.dmtf.org/wbem/cim-html/2.49.0/CIM_AssociatedPowerManagementService.html
// see https://software.intel.com/sites/manageability/AMT_Implementation_and_Reference_Guide/default.htm?turl=HTMLDocuments%2FWS-Management_Class_Reference%2FCIM_AssociatedPowerManagementService.htm
// see https://github.com/rgl/intel-amt-toggle-power-example/blob/master/intel-amt-toggle-power-example.js
// see https://github.com/device-management-toolkit/go-wsman-messages/issues/377
func main() {
	host := flag.String("host", "192.168.8.22", "Intel AMT host or address")
	username := flag.String("username", "admin", "Intel AMT username")
	password := flag.String("password", os.Getenv("AMT_PASSWORD"), "Intel AMT password (defaults to the value of the AMT_PASSWORD environment variable)")
	useDigest := flag.Bool("use-digest", true, "use HTTP digest authentication")
	useTLS := flag.Bool("use-tls", false, "use TLS")
	selfSignedAllowed := flag.Bool("self-signed-allowed", true, "allow self-signed TLS certificates")
	logAMTMessages := flag.Bool("log-amt-messages", false, "log AMT messages")
	flag.Parse()

	if *host == "" {
		log.Fatal("--host must be set")
	}

	if *username == "" {
		log.Fatal("--username must be set")
	}

	if *password == "" {
		log.Fatal("--password or the AMT_PASSWORD environment variable must be set")
	}

	clientParameters := client.Parameters{
		Target:            *host,
		Username:          *username,
		Password:          *password,
		UseDigest:         *useDigest,
		UseTLS:            *useTLS,
		SelfSignedAllowed: *selfSignedAllowed,
		LogAMTMessages:    *logAMTMessages,
	}
	messages := wsman.NewMessages(clientParameters)
	defer messages.Client.CloseConnection()

	// get the current power state.
	response, err := messages.CIM.AssociatedPowerManagementService.Get()
	if err != nil {
		log.Fatalf("failed to get the power state: %v", err)
	}
	if *logAMTMessages {
		fmt.Println("Current Power State Response:")
		responseXML, err := formatXML(response.XMLOutput)
		if err != nil {
			responseXML = response.XMLOutput
		}
		fmt.Println(responseXML)
		fmt.Println(response.YAML())
	}
	log.Printf("Current power state: %s", response.Body.AssociatedPowerManagementService.PowerState)
	for _, availablePowerState := range response.Body.AssociatedPowerManagementService.AvailableRequestedPowerStates {
		log.Printf("Available power state: %s", availablePowerState)
	}

	// toggle to the desired power state.
	// NB RequestPowerStateChange can set a powerState that is in availablePowerStates array.
	// NB PowerStateOffSoft will abruptly power down the system.
	// NB PowerStateOffSoftGraceful will gracefully power down the system.
	//    NB You must have Intel LMS (Local Manageability Service) running in the OS.
	//       See https://github.com/intel/lms
	// NB A linux based system was gracefully shutdown when the `last` command
	//    does not contain a `crash` entry.
	var powerState = response.Body.AssociatedPowerManagementService.PowerState
	var togglePowerState models.PowerState
	var desiredPowerState models.PowerState
	if powerState == models.PowerStateOn {
		// try to power off gracefully if available, otherwise, just power it off abruptly.
		// NB To power off gracefully the system should have the LMS running in the OS.
		// NB After a power off, you should wait for a bit, until the LMS starts running,
		//    or else, the system will be powered off abruptly. The MeshCommander UI will
		//    just make the user wait for 30s or so before allowing the user to continue.
		//    So, this means, we should check the system power up time, and only after
		//    X minutes of being powered on, we would allow/consider abruptly powering
		//    off the system.
		//    TODO see if there's an API that returns how long the system has power.
		// NB A graceful stop is normally delayed by LMS, that is, LMS schedules a
		//    shutdown to happen in the near future (e.g. after 1m).
		//    TODO see how can we configure that behavior.
		//    In my case, this was seen with journalctl:
		//          Broadcast message from root@vagrant (Tue 2020-08-18 22:00:40 WEST):
		//          The remote administrator has initiated a shutdown on this computer...
		//          The system is going down for poweroff at Tue 2020-08-18 22:01:40 WEST!
		//          ago 18 22:00:40 vagrant systemd-logind[574]: Creating /run/nologin, blocking further logins...
		//          ago 18 22:00:40 vagrant LMS[568]: Remote administrator shutdown request was executed
		// NB when we request a PowerStateOffSoftGraceful transition, AMT will actually report
		//    PowerStateOffSoft when its done.
		togglePowerState = models.PowerStateOffSoft
		for _, availablePowerState := range response.Body.AssociatedPowerManagementService.AvailableRequestedPowerStates {
			if availablePowerState == models.AvailableRequestedPowerStatesPowerOffSoftGraceful {
				togglePowerState = models.PowerStatePowerOffSoftGraceful
				break
			}
		}
		desiredPowerState = models.PowerStateOffSoft
	} else {
		togglePowerState = models.PowerStateOn
		desiredPowerState = models.PowerStateOn
	}
	togglePowerStateAvailable := false
	for _, availablePowerState := range response.Body.AssociatedPowerManagementService.AvailableRequestedPowerStates {
		if models.PowerState(availablePowerState) == togglePowerState {
			togglePowerStateAvailable = true
			break
		}
	}
	if !togglePowerStateAvailable {
		log.Fatalf("the desired power state is not available: %v", err)
	}
	log.Printf("Toggling the power state from %s to %s", powerState, togglePowerState)
	powerStateChangeResponse, err := messages.CIM.PowerManagementService.RequestPowerStateChange(power.PowerState(togglePowerState))
	if err != nil {
		log.Fatalf("failed to set the power state: %v", err)
	}
	if *logAMTMessages {
		fmt.Println("Change Power State Response:")
		powerStateChangeResponseXML, err := formatXML(powerStateChangeResponse.XMLOutput)
		if err != nil {
			powerStateChangeResponseXML = powerStateChangeResponse.XMLOutput
		}
		fmt.Println(powerStateChangeResponseXML)
		fmt.Println(powerStateChangeResponse.YAML())
	}
	if powerStateChangeResponse.Body.RequestPowerStateChangeResponse.ReturnValue != 0 {
		log.Fatalf("failed to set the power state. return value was %d.", powerStateChangeResponse.Body.RequestPowerStateChangeResponse.ReturnValue)
	}

	// wait for the desired power state.
	log.Printf("Waiting for the power state to be %s", desiredPowerState)
	for lastPowerState := models.PowerState(0); ; {
		response, err := messages.CIM.AssociatedPowerManagementService.Get()
		if err != nil {
			log.Fatalf("failed to get the power state: %v", err)
		}
		powerState := response.Body.AssociatedPowerManagementService.PowerState
		if powerState == desiredPowerState {
			log.Printf("The power state is now %s", powerState)
			break
		}
		if powerState != lastPowerState {
			log.Printf("Waiting for the power state to change from %s to %s...", powerState, desiredPowerState)
			lastPowerState = powerState
		}
		time.Sleep(1 * time.Second)
	}
}

func formatXML(sourceXML string) (string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command("xmllint", "--format", "-")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = bytes.NewBufferString(sourceXML)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("xmllint error: %v, stderr: %s", err, stderr.String())
	}
	formattedXML := strings.TrimSpace(stdout.String())
	return formattedXML, nil
}
