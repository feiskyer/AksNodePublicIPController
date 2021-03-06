package helpers

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2018-10-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2017-09-01/network"
	"github.com/Azure/go-autorest/autorest/to"
	log "github.com/Sirupsen/logrus"
)

var (
	scaleSetNameRE = regexp.MustCompile(`.*/subscriptions/(?:.*)/Microsoft.Compute/virtualMachineScaleSets/(.+)/virtualMachines(?:.*)`)
)

func extractScaleSetNameByProviderID(providerID string) (string, error) {
	matches := scaleSetNameRE.FindStringSubmatch(providerID)
	if len(matches) != 2 {
		return "", fmt.Errorf("not a vmss instance")
	}

	return matches[1], nil
}

func getIPClient() network.PublicIPAddressesClient {
	ipClient := network.NewPublicIPAddressesClient(spDetails.SubscriptionID)
	auth, _ := GetResourceManagementAuthorizer()
	ipClient.Authorizer = auth
	return ipClient
}

func getVMClient() compute.VirtualMachinesClient {
	vmClient := compute.NewVirtualMachinesClient(spDetails.SubscriptionID)
	auth, _ := GetResourceManagementAuthorizer()
	vmClient.Authorizer = auth
	return vmClient
}

func getNicClient() network.InterfacesClient {
	nicClient := network.NewInterfacesClient(spDetails.SubscriptionID)
	auth, _ := GetResourceManagementAuthorizer()
	nicClient.Authorizer = auth
	return nicClient
}

func getVmssClient() compute.VirtualMachineScaleSetsClient {
	vmssClient := compute.NewVirtualMachineScaleSetsClient(spDetails.SubscriptionID)
	auth, _ := GetResourceManagementAuthorizer()
	vmssClient.Authorizer = auth
	return vmssClient
}

func createPublicIP(ctx context.Context, ipName string) (ip network.PublicIPAddress, err error) {
	ipClient := getIPClient()
	future, err := ipClient.CreateOrUpdate(
		ctx,
		spDetails.ResourceGroup,
		ipName,
		network.PublicIPAddress{
			Name:     to.StringPtr(ipName),
			Location: &spDetails.Location,
			PublicIPAddressPropertiesFormat: &network.PublicIPAddressPropertiesFormat{
				PublicIPAddressVersion:   network.IPv4,
				PublicIPAllocationMethod: network.Dynamic, // IPv4 address created is a dynamic one
			},
		},
	)

	if err != nil {
		return ip, fmt.Errorf("cannot create Public IP address: %v", err)
	}

	err = future.WaitForCompletion(ctx, ipClient.Client)
	if err != nil {
		return ip, fmt.Errorf("cannot get Public IP address CreateOrUpdate method response: %v", err)
	}

	return future.Result(ipClient)
}

func getVM(ctx context.Context, vmName string) (compute.VirtualMachine, error) {
	vmClient := getVMClient()
	return vmClient.Get(ctx, spDetails.ResourceGroup, vmName, compute.InstanceView)
}

func getNetworkInterface(ctx context.Context, vmName string) (*network.Interface, error) {
	vm, err := getVM(ctx, vmName)
	if err != nil {
		return nil, err
	}

	//this will be something like /subscriptions/6bd0e514-c783-4dac-92d2-6788744eee7a/resourceGroups/MC_akslala_akslala_westeurope/providers/Microsoft.Network/networkInterfaces/aks-nodepool1-26427378-nic-0
	nicFullName := &(*vm.NetworkProfile.NetworkInterfaces)[0].ID

	nicName := getResourceName(**nicFullName)

	nicClient := getNicClient()

	networkInterface, err := nicClient.Get(ctx, spDetails.ResourceGroup, nicName, "")
	return &networkInterface, err
}

type IPUpdater interface {
	CreateOrUpdateVMPulicIP(ctx context.Context, vmName, providerID string, ipName string) error
	DeletePublicIP(ctx context.Context, ipName string) error
	DisassociatePublicIPForNode(ctx context.Context, nodeName, providerID string) error
	UpdateVMSSPublicIP(ctx context.Context, scaleSet string) error
}

type IPUpdate struct{}

// CreateOrUpdateVMPulicIP will create a new Public IP and assign it to the Virtual Machine
func (u *IPUpdate) CreateOrUpdateVMPulicIP(ctx context.Context, vmName, providerID string, ipName string) error {
	log.Infof("Trying to get NIC from the node %q", vmName)

	scaleSet, err := extractScaleSetNameByProviderID(providerID)
	if err == nil {
		log.Infof("Trying to setup public IP per instance for scale set %q", scaleSet)
		return u.UpdateVMSSPublicIP(ctx, scaleSet)
	}

	nic, err := getNetworkInterface(ctx, vmName)
	if err != nil {
		return fmt.Errorf("cannot get network interface: %v", err)
	}

	log.Infof("Trying to create the Public IP for Node %s", vmName)

	ip, err := createPublicIP(ctx, ipName)
	if err != nil {
		return fmt.Errorf("Cannot create Public IP for Node %s: %v", vmName, err)
	}

	log.Infof("Public IP for Node %s created", vmName)

	// set this IP Adress to NIC's IP configuration
	(*nic.IPConfigurations)[0].PublicIPAddress = &ip

	nicClient := getNicClient()

	log.Infof("Trying to assign the Public IP to the NIC for Node %s", vmName)

	future, err := nicClient.CreateOrUpdate(ctx, spDetails.ResourceGroup, getResourceName(*nic.ID), *nic)

	if err != nil {
		return fmt.Errorf("cannot update NIC for Node %s: %v", vmName, err)
	}

	err = future.WaitForCompletion(ctx, nicClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get NIC CreateOrUpdate response for Node %s: %v", vmName, err)
	}

	log.Infof("NIC for Node %s successfully updated", vmName)

	return nil
}

// DeletePublicIP deletes the designated Public IP
func (*IPUpdate) DeletePublicIP(ctx context.Context, ipName string) error {
	ipClient := getIPClient()
	future, err := ipClient.Delete(ctx, spDetails.ResourceGroup, ipName)
	if err != nil {
		return fmt.Errorf("cannot delete Public IP address %s: %v", ipName, err)
	}

	err = future.WaitForCompletion(ctx, ipClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get public ip address %s CreateOrUpdate method's response: %v", ipName, err)
	}

	log.Infof("IP %s successfully deleted", ipName)

	return nil
}

// DisassociatePublicIPForNode will remove the Public IP address association from the VM's NIC
func (*IPUpdate) DisassociatePublicIPForNode(ctx context.Context, nodeName, providerID string) error {
	_, err := extractScaleSetNameByProviderID(providerID)
	if err == nil {
		log.Infof("Skipping public IP deallocating for node %q since it is managed by VMSS", nodeName)
		return nil
	}

	ipClient := getIPClient()
	ipAddress, err := ipClient.Get(ctx, spDetails.ResourceGroup, GetPublicIPName(nodeName), "")
	if err != nil {
		return fmt.Errorf("cannot get IP Address: %v for Node %s", err, nodeName)
	}

	var nicName string
	if ipAddress.IPConfiguration != nil {
		ipConfiguration := *ipAddress.IPConfiguration.ID
		//ipConfiguration has a value similar to:
		///subscriptions/X/resourceGroups/Y/providers/Microsoft.Network/networkInterfaces/aks-nodepool1-26427378-nic-X/ipConfigurations/ipconfig1

		nicName = getNICNameFromIPConfiguration(ipConfiguration)
	} else {
		// IPConfiguration is nil => this IP address is already disassociated
		return nil
	}

	nicClient := getNicClient()

	// get the NIC
	nic, err := nicClient.Get(ctx, spDetails.ResourceGroup, nicName, "")
	if err != nil {
		return fmt.Errorf("cannot get NIC for Node %s, error: %v", nodeName, err)
	}

	// set its Public IP to nil
	(*nic.IPConfigurations)[0].PublicIPAddress = nil

	// update the NIC so it has a nil Public IP
	future, err := nicClient.CreateOrUpdate(ctx, spDetails.ResourceGroup, getResourceName(*nic.ID), nic)

	if err != nil {
		return fmt.Errorf("cannot update NIC for Node %s, error: %v", nodeName, err)
	}

	err = future.WaitForCompletion(ctx, nicClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get NIC CreateOrUpdate response for Node %s, error: %v", nodeName, err)
	}

	// there is a chance that after the scale-in operation completes, the NIC will still be alive
	// This may happen due to a race condition between AKS calling Delete on the NIC and our code that
	// calls CreateOrUpdate
	// to make sure NIC gets removed, we'll just call delete on its instance
	futureDelete, err := nicClient.Delete(ctx, spDetails.ResourceGroup, getResourceName(*nic.ID))
	if err != nil {
		return fmt.Errorf("cannot delete NIC for Node %s, error: %v. NIC may have already been deleted", nodeName, err)
	}

	err = futureDelete.WaitForCompletion(ctx, nicClient.Client)
	if err != nil {
		return fmt.Errorf("cannot get NIC Delete response for Node %s:, error: %v. NIC may have already been deleted", nodeName, err)
	}

	return nil
}

func (u *IPUpdate) UpdateVMSSPublicIP(ctx context.Context, scaleSet string) error {
	vmssClient := getVmssClient()
	vmss, err := vmssClient.Get(ctx, spDetails.ResourceGroup, scaleSet)
	if err != nil {
		return fmt.Errorf("failed to get vmss %q: %v", scaleSet, err)
	}

	// Setup public IP per virtual machine for VMSS.
	if vmss.VirtualMachineProfile != nil && vmss.VirtualMachineProfile.NetworkProfile != nil &&
		vmss.VirtualMachineProfile.NetworkProfile.NetworkInterfaceConfigurations != nil {
		ifaces := *vmss.VirtualMachineProfile.NetworkProfile.NetworkInterfaceConfigurations
		if len(ifaces) > 0 && ifaces[0].IPConfigurations != nil {
			ips := *ifaces[0].IPConfigurations
			ips[0].PublicIPAddressConfiguration = &compute.VirtualMachineScaleSetPublicIPAddressConfiguration{
				Name: to.StringPtr(scaleSet + "_public_ip"),
			}

			// Update VMSS.
			ifaces[0].IPConfigurations = &ips
			vmss.VirtualMachineProfile.NetworkProfile.NetworkInterfaceConfigurations = &ifaces
			future, err := vmssClient.CreateOrUpdate(ctx, spDetails.ResourceGroup, scaleSet, vmss)
			if err != nil {
				return fmt.Errorf("failed to CreateOrUpdate vmss %q: %v", scaleSet, err)
			}
			err = future.WaitForCompletionRef(ctx, vmssClient.Client)
			if err != nil {
				return fmt.Errorf("failed to update VMSS %q: %v", scaleSet, err)
			}

			// Update VMSS instances.
			instanceIDs := []string{"*"}
			instanceFuture, err := vmssClient.UpdateInstances(ctx, spDetails.ResourceGroup, scaleSet, compute.VirtualMachineScaleSetVMInstanceRequiredIDs{
				InstanceIds: &instanceIDs,
			})
			if err != nil {
				return fmt.Errorf("failed to update instances for vmss %q: %v", scaleSet, err)
			}
			err = instanceFuture.WaitForCompletionRef(ctx, vmssClient.Client)
			if err != nil {
				return fmt.Errorf("failed to update instances for vmss %q: %v", scaleSet, err)
			}
		}
	}

	return nil
}

// getResourceName accepts a string of type
// /subscriptions/A/resourceGroups/B/providers/Microsoft.Network/publicIPAddresses/ipconfig-aks-nodepool1-X
// will return just the ID, i.e. ipconfig-aks-nodepool1-X
func getResourceName(fullID string) string {
	parts := strings.Split(fullID, "/")
	return parts[len(parts)-1]
}

func getNICNameFromIPConfiguration(ipConfig string) string {
	///subscriptions/X/resourceGroups/Y/providers/Microsoft.Network/networkInterfaces/aks-nodepool1-26427378-nic-X/ipConfigurations/ipconfig1
	parts := strings.Split(ipConfig, "/")
	return parts[len(parts)-3]
}
