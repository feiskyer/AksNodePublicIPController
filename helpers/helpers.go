// Copyright (c) Microsoft and contributors.  All rights reserved.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package helpers

import (
	"io/ioutil"
	"sigs.k8s.io/yaml"
)

// ServicePrincipalDetails contains the Service Principal credentials for the AKS cluster
type ServicePrincipalDetails struct {
	// The cloud environment identifier. Takes values from https://github.com/Azure/go-autorest/blob/ec5f4903f77ed9927ac95b19ab8e44ada64c1356/autorest/azure/environments.go#L13
	Cloud string `json:"cloud" yaml:"cloud"`
	// The AAD Tenant ID for the Subscription that the cluster is deployed in
	TenantID string `json:"tenantId" yaml:"tenantId"`
	// The ClientID for an AAD application with RBAC access to talk to Azure RM APIs
	AADClientID string `json:"aadClientId" yaml:"aadClientId"`
	// The ClientSecret for an AAD application with RBAC access to talk to Azure RM APIs
	AADClientSecret string `json:"aadClientSecret" yaml:"aadClientSecret"`
	// The path of a client certificate for an AAD application with RBAC access to talk to Azure RM APIs
	AADClientCertPath string `json:"aadClientCertPath" yaml:"aadClientCertPath"`
	// The password of the client certificate for an AAD application with RBAC access to talk to Azure RM APIs
	AADClientCertPassword string `json:"aadClientCertPassword" yaml:"aadClientCertPassword"`
	// Use managed service identity for the virtual machine to access Azure ARM APIs
	UseManagedIdentityExtension bool `json:"useManagedIdentityExtension" yaml:"useManagedIdentityExtension"`
	// UserAssignedIdentityID contains the Client ID of the user assigned MSI which is assigned to the underlying VMs. If empty the user assigned identity is not used.
	// More details of the user assigned identity can be found at: https://docs.microsoft.com/en-us/azure/active-directory/managed-service-identity/overview
	// For the user assigned identity specified here to be used, the UseManagedIdentityExtension has to be set to true.
	UserAssignedIdentityID string `json:"userAssignedIdentityID" yaml:"userAssignedIdentityID"`
	// The ID of the Azure Subscription that the cluster is deployed in
	SubscriptionID string `json:"subscriptionId" yaml:"subscriptionId"`
	// The location of the resource group that the cluster is deployed in
	Location string `json:"location" yaml:"location"`
	// The name of the resource group that the cluster is deployed in
	ResourceGroup string `json:"resourceGroup" yaml:"resourceGroup"`
}

var spDetails ServicePrincipalDetails

/*
/etc/kubernetes/azure.json is ...

{
    "cloud":"AzurePublicCloud",
    "tenantId": "XXX",
    "subscriptionId": "XXX",
    "aadClientId": "XXXX",
    "aadClientSecret": "XXXXX",
    "resourceGroup": "MC_akslala_akslala_westeurope",
    "location": "westeurope",
	...
}
*/

// InitializeServicePrincipalDetails reads the /etc/kubernetes/azure.json file on the host (mounted via hostPath on the Pod)
// this files contains the credentials for the AKS cluster's Service Principal
func InitializeServicePrincipalDetails() error {
	file, err := ioutil.ReadFile("/akssp/azure.json")
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(file, &spDetails)
	if err != nil {
		return err
	}

	return nil
}

// GetPublicIPName returns the name of the Public IP resource, which is based on the Node's name
func GetPublicIPName(vmName string) string {
	return "ipconfig-" + vmName
}
