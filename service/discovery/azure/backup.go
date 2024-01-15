// Copyright 2024 Fraunhofer AISEC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
//           $$\                           $$\ $$\   $$\
//           $$ |                          $$ |\__|  $$ |
//  $$$$$$$\ $$ | $$$$$$\  $$\   $$\  $$$$$$$ |$$\ $$$$$$\    $$$$$$\   $$$$$$\
// $$  _____|$$ |$$  __$$\ $$ |  $$ |$$  __$$ |$$ |\_$$  _|  $$  __$$\ $$  __$$\
// $$ /      $$ |$$ /  $$ |$$ |  $$ |$$ /  $$ |$$ |  $$ |    $$ /  $$ |$$ | \__|
// $$ |      $$ |$$ |  $$ |$$ |  $$ |$$ |  $$ |$$ |  $$ |$$\ $$ |  $$ |$$ |
// \$$$$$$\  $$ |\$$$$$   |\$$$$$   |\$$$$$$  |$$ |  \$$$   |\$$$$$   |$$ |
//  \_______|\__| \______/  \______/  \_______|\__|   \____/  \______/ \__|
//
// This file is part of Clouditor Community Edition.

package azure

import (
	"context"
	"errors"
	"fmt"

	"clouditor.io/clouditor/internal/constants"
	"clouditor.io/clouditor/internal/util"
	"clouditor.io/clouditor/voc"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/dataprotection/armdataprotection"
)

// discoverBackupVaults receives all backup vaults in the subscription.
// Since the backups for storage and compute are discovered together, the discovery is performed here and results are stored in the azureDiscovery receiver.
func (d *azureDiscovery) discoverBackupVaults() error {

	if d.backupMap != nil && len(d.backupMap) > 0 {
		log.Debug("Backup Vaults already discovered.")
		return nil
	}

	// initialize backup vaults client
	if err := d.initBackupVaultsClient(); err != nil {
		return err
	}

	// initialize backup instances client
	if err := d.initBackupInstancesClient(); err != nil {
		return err
	}

	// initialize backup policies client
	if err := d.initBackupPoliciesClient(); err != nil {
		return err
	}

	// List all backup vaults
	err := listPager(d,
		d.clients.backupVaultClient.NewGetInSubscriptionPager,
		d.clients.backupVaultClient.NewGetInResourceGroupPager,
		func(res armdataprotection.BackupVaultsClientGetInSubscriptionResponse) []*armdataprotection.BackupVaultResource {
			return res.Value
		},
		func(res armdataprotection.BackupVaultsClientGetInResourceGroupResponse) []*armdataprotection.BackupVaultResource {
			return res.Value
		},
		func(vault *armdataprotection.BackupVaultResource) error {
			instances, err := d.discoverBackupInstances(resourceGroupName(util.Deref(vault.ID)), util.Deref(vault.Name))
			if err != nil {
				err := fmt.Errorf("could not discover backup instances: %v", err)
				return err
			}

			for _, instance := range instances {
				dataSourceType := util.Deref(instance.Properties.DataSourceInfo.DatasourceType)

				// Get retention from backup policy
				policy, err := d.clients.backupPoliciesClient.Get(context.Background(), resourceGroupName(*vault.ID), *vault.Name, backupPolicyName(*instance.Properties.PolicyInfo.PolicyID), &armdataprotection.BackupPoliciesClientGetOptions{})
				if err != nil {
					err := fmt.Errorf("could not get backup policy '%s': %w", *instance.Properties.PolicyInfo.PolicyID, err)
					log.Error(err)
					continue
				}

				// TODO(all):Maybe we should differentiate the backup retention period for different resources, e.g., disk vs blobs (Metrics)
				retention := policy.BaseBackupPolicyResource.Properties.(*armdataprotection.BackupPolicy).PolicyRules[0].(*armdataprotection.AzureRetentionRule).Lifecycles[0].DeleteAfter.(*armdataprotection.AbsoluteDeleteOption).GetDeleteOption().Duration

				resp, err := d.handleInstances(vault, instance)
				if err != nil {
					err := fmt.Errorf("could not handle instance")
					return err
				}

				// Check if map entry already exists
				_, ok := d.backupMap[dataSourceType]
				if !ok {
					d.backupMap[dataSourceType] = &backup{
						backup: make(map[string][]*voc.Backup),
					}
				}

				// Store voc.Backup in backupMap
				d.backupMap[dataSourceType].backup[util.Deref(instance.Properties.DataSourceInfo.ResourceID)] = []*voc.Backup{
					{
						Enabled:         true,
						RetentionPeriod: retentionDuration(util.Deref(retention)),
						Storage:         voc.ResourceID(util.Deref(instance.ID)),
						TransportEncryption: &voc.TransportEncryption{
							Enabled:    true,
							Enforced:   true,
							Algorithm:  constants.TLS,
							TlsVersion: constants.TLS1_2, // https://learn.microsoft.com/en-us/azure/backup/transport-layer-security#why-enable-tls-12 (Last access: 04/27/2023)
						},
					},
				}

				// Store backed up storage voc objects (ObjectStorage, BlockStorage)
				d.backupMap[dataSourceType].backupStorages = append(d.backupMap[dataSourceType].backupStorages, resp)
			}
			return nil
		})

	if err != nil {
		return err
	}

	return nil
}

// discoverBackupInstances retrieves the instances in a given backup vault.
// Note: It is only possible to backup a storage account with all containers in it.
func (d *azureDiscovery) discoverBackupInstances(resourceGroup, vaultName string) ([]*armdataprotection.BackupInstanceResource, error) {
	var (
		list armdataprotection.BackupInstancesClientListResponse
		err  error
	)

	if resourceGroup == "" || vaultName == "" {
		return nil, errors.New("missing resource group and/or vault name")
	}

	// List all instances in the given backup vault
	listPager := d.clients.backupInstancesClient.NewListPager(resourceGroup, vaultName, &armdataprotection.BackupInstancesClientListOptions{})
	for listPager.More() {
		list, err = listPager.NextPage(context.TODO())
		if err != nil {
			err = fmt.Errorf("%s: %v", ErrGettingNextPage, err)
			return nil, err
		}
	}

	return list.Value, nil
}

func (d *azureDiscovery) handleInstances(vault *armdataprotection.BackupVaultResource, instance *armdataprotection.BackupInstanceResource) (resource voc.IsCloudResource, err error) {
	if vault == nil || instance == nil {
		return nil, ErrVaultInstanceIsEmpty
	}

	raw, err := voc.ToStringInterface([]interface{}{instance, vault})
	if err != nil {
		log.Error(err)
	}

	if *instance.Properties.DataSourceInfo.DatasourceType == "Microsoft.Storage/storageAccounts/blobServices" {
		resource = &voc.ObjectStorage{
			Storage: &voc.Storage{
				Resource: &voc.Resource{
					ID:           voc.ResourceID(*instance.ID),
					Name:         *instance.Name,
					CreationTime: 0,
					GeoLocation: voc.GeoLocation{
						Region: *vault.Location,
					},
					Labels:    nil,
					ServiceID: d.csID,
					Type:      voc.ObjectStorageType,
					Parent:    resourceGroupID(instance.ID),
					Raw:       raw,
				},
			},
		}
	} else if *instance.Properties.DataSourceInfo.DatasourceType == "Microsoft.Compute/disks" {
		resource = &voc.BlockStorage{
			Storage: &voc.Storage{
				Resource: &voc.Resource{
					ID:           voc.ResourceID(*instance.ID),
					Name:         *instance.Name,
					ServiceID:    d.csID,
					CreationTime: 0,
					Type:         voc.BlockStorageType,
					GeoLocation: voc.GeoLocation{
						Region: *vault.Location,
					},
					Labels: nil,
					Parent: resourceGroupID(instance.ID),
					Raw:    raw,
				},
			},
		}
	}

	return
}

// backupsEmptyCheck checks if the backups list is empty and returns voc.Backup with enabled = false.
func backupsEmptyCheck(backups []*voc.Backup) []*voc.Backup {
	if len(backups) == 0 {
		return []*voc.Backup{
			{
				Enabled:         false,
				RetentionPeriod: -1,
				Interval:        -1,
			},
		}
	}

	return backups
}