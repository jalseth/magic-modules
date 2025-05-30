// ----------------------------------------------------------------------------
//
//	This file is copied here by Magic Modules. The code for building up a
//	sql database instance object is copied from the manually implemented
//	provider file:
//	third_party/tgc/services/sql/sql_database_instance.go
//
// ----------------------------------------------------------------------------
package sql

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/id"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/GoogleCloudPlatform/terraform-google-conversion/v6/tfplan2cai/converters/google/resources/cai"
	"github.com/hashicorp/terraform-provider-google-beta/google-beta/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google-beta/google-beta/transport"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
)

const SQLDatabaseInstanceAssetType string = "sqladmin.googleapis.com/Instance"

func ResourceConverterSQLDatabaseInstance() cai.ResourceConverter {
	return cai.ResourceConverter{
		AssetType: SQLDatabaseInstanceAssetType,
		Convert:   GetSQLDatabaseInstanceCaiObject,
	}
}

func GetSQLDatabaseInstanceCaiObject(d tpgresource.TerraformResourceData, config *transport_tpg.Config) ([]cai.Asset, error) {
	name, err := cai.AssetName(d, config, "//cloudsql.googleapis.com/projects/{{project}}/instances/{{name}}")
	if err != nil {
		return []cai.Asset{}, err
	}
	if obj, err := GetSQLDatabaseInstanceApiObject(d, config); err == nil {
		return []cai.Asset{{
			Name: name,
			Type: SQLDatabaseInstanceAssetType,
			Resource: &cai.AssetResource{
				Version:              "v1beta4",
				DiscoveryDocumentURI: "https://www.googleapis.com/discovery/v1/apis/sqladmin/v1beta4/rest",
				DiscoveryName:        "DatabaseInstance",
				Data:                 obj,
			},
		}}, nil
	} else {
		return []cai.Asset{}, err
	}
}

func GetSQLDatabaseInstanceApiObject(d tpgresource.TerraformResourceData, config *transport_tpg.Config) (map[string]interface{}, error) {
	project, err := tpgresource.GetProject(d, config)
	if err != nil {
		return nil, err
	}

	region, err := tpgresource.GetRegion(d, config)
	if err != nil {
		return nil, err
	}

	var name string
	if v, ok := d.GetOk("name"); ok {
		name = v.(string)
	} else {
		name = id.UniqueId()
	}

	instance := &sqladmin.DatabaseInstance{
		Project:              project,
		Name:                 name,
		Region:               region,
		Settings:             expandSqlDatabaseInstanceSettings(d.Get("settings").([]interface{}), !isFirstGen(d)),
		DatabaseVersion:      safeString(d.Get("database_version")),
		MasterInstanceName:   safeString(d.Get("master_instance_name")),
		ReplicaConfiguration: expandReplicaConfiguration(d.Get("replica_configuration").([]interface{})),
	}

	return cai.JsonMap(instance)
}

// Detects whether a database is 1st Generation by inspecting the tier name
func isFirstGen(d tpgresource.TerraformResourceData) bool {
	settingsList := d.Get("settings").([]interface{})
	settings := settingsList[0].(map[string]interface{})
	tier := safeString(settings["tier"])

	// 1st Generation databases have tiers like 'D0', as opposed to 2nd Generation which are
	// prefixed with 'db'
	return !regexp.MustCompile("db*").Match([]byte(tier))
}

func expandSqlDatabaseInstanceSettings(configured []interface{}, secondGen bool) *sqladmin.Settings {
	if len(configured) == 0 || configured[0] == nil {
		return nil
	}

	_settings := configured[0].(map[string]interface{})
	settings := &sqladmin.Settings{
		// Version is unset in Create but is set during update
		SettingsVersion:     int64(safeInt(_settings["version"])),
		Tier:                safeString(_settings["tier"]),
		ForceSendFields:     []string{"StorageAutoResize"},
		ActivationPolicy:    safeString(_settings["activation_policy"]),
		AvailabilityType:    safeString(_settings["availability_type"]),
		DataDiskSizeGb:      int64(safeInt(_settings["disk_size"])),
		DataDiskType:        safeString(_settings["disk_type"]),
		PricingPlan:         safeString(_settings["pricing_plan"]),
		UserLabels:          tpgresource.ConvertStringMap(_settings["user_labels"].(map[string]interface{})),
		BackupConfiguration: expandBackupConfiguration(_settings["backup_configuration"].([]interface{})),
		DatabaseFlags:       expandDatabaseFlags(_settings["database_flags"].(*schema.Set).List()),
		IpConfiguration:     expandIpConfiguration(_settings["ip_configuration"].([]interface{})),
		LocationPreference:  expandLocationPreference(_settings["location_preference"].([]interface{})),
		MaintenanceWindow:   expandMaintenanceWindow(_settings["maintenance_window"].([]interface{})),
	}

	// 1st Generation instances don't support the disk_autoresize parameter
	// and it defaults to true - so we shouldn't set it if this is first gen
	if secondGen {
		diskAutoresize := safeBool(_settings["disk_autoresize"])
		settings.StorageAutoResize = &diskAutoresize
		settings.StorageAutoResizeLimit = int64(safeInt(_settings["disk_autoresize_limit"]))
	}

	return settings
}

func expandReplicaConfiguration(configured []interface{}) *sqladmin.ReplicaConfiguration {
	if len(configured) == 0 || configured[0] == nil {
		return nil
	}

	_replicaConfiguration := configured[0].(map[string]interface{})
	return &sqladmin.ReplicaConfiguration{
		FailoverTarget: safeBool(_replicaConfiguration["failover_target"]),

		// MysqlReplicaConfiguration has been flattened in the TF schema, so
		// we'll keep it flat here instead of another expand method.
		MysqlReplicaConfiguration: &sqladmin.MySqlReplicaConfiguration{
			CaCertificate:           safeString(_replicaConfiguration["ca_certificate"]),
			ClientCertificate:       safeString(_replicaConfiguration["client_certificate"]),
			ClientKey:               safeString(_replicaConfiguration["client_key"]),
			ConnectRetryInterval:    int64(safeInt(_replicaConfiguration["connect_retry_interval"])),
			DumpFilePath:            safeString(_replicaConfiguration["dump_file_path"]),
			MasterHeartbeatPeriod:   int64(safeInt(_replicaConfiguration["master_heartbeat_period"])),
			Password:                safeString(_replicaConfiguration["password"]),
			SslCipher:               safeString(_replicaConfiguration["ssl_cipher"]),
			Username:                safeString(_replicaConfiguration["username"]),
			VerifyServerCertificate: safeBool(_replicaConfiguration["verify_server_certificate"]),
		},
	}
}

func expandMaintenanceWindow(configured []interface{}) *sqladmin.MaintenanceWindow {
	if len(configured) == 0 || configured[0] == nil {
		return nil
	}

	window := configured[0].(map[string]interface{})
	return &sqladmin.MaintenanceWindow{
		Day:             int64(safeInt(window["day"])),
		Hour:            int64(safeInt(window["hour"])),
		UpdateTrack:     safeString(window["update_track"]),
		ForceSendFields: []string{"Hour"},
	}
}

func expandLocationPreference(configured []interface{}) *sqladmin.LocationPreference {
	if len(configured) == 0 || configured[0] == nil {
		return nil
	}

	_locationPreference := configured[0].(map[string]interface{})
	return &sqladmin.LocationPreference{
		FollowGaeApplication: safeString(_locationPreference["follow_gae_application"]),
		Zone:                 safeString(_locationPreference["zone"]),
		SecondaryZone:        safeString(_locationPreference["secondary_zone"]),
	}
}

func expandIpConfiguration(configured []interface{}) *sqladmin.IpConfiguration {
	if len(configured) == 0 || configured[0] == nil {
		return nil
	}

	_ipConfiguration := configured[0].(map[string]interface{})

	return &sqladmin.IpConfiguration{
		Ipv4Enabled:        safeBool(_ipConfiguration["ipv4_enabled"]),
		PrivateNetwork:     safeString(_ipConfiguration["private_network"]),
		AuthorizedNetworks: expandAuthorizedNetworks(_ipConfiguration["authorized_networks"].(*schema.Set).List()),
		ForceSendFields:    []string{"Ipv4Enabled"},
		NullFields:         []string{"RequireSsl"},
		SslMode:            safeString(_ipConfiguration["ssl_mode"]),
	}
}
func expandAuthorizedNetworks(configured []interface{}) []*sqladmin.AclEntry {
	an := make([]*sqladmin.AclEntry, 0, len(configured))
	for _, _acl := range configured {
		_entry := _acl.(map[string]interface{})
		an = append(an, &sqladmin.AclEntry{
			ExpirationTime: safeString(_entry["expiration_time"]),
			Name:           safeString(_entry["name"]),
			Value:          safeString(_entry["value"]),
		})
	}

	return an
}

func expandDatabaseFlags(configured []interface{}) []*sqladmin.DatabaseFlags {
	databaseFlags := make([]*sqladmin.DatabaseFlags, 0, len(configured))
	for _, _flag := range configured {
		_entry := _flag.(map[string]interface{})

		databaseFlags = append(databaseFlags, &sqladmin.DatabaseFlags{
			Name:  safeString(_entry["name"]),
			Value: safeString(_entry["value"]),
		})
	}
	return databaseFlags
}

func expandBackupConfiguration(configured []interface{}) *sqladmin.BackupConfiguration {
	if len(configured) == 0 || configured[0] == nil {
		return nil
	}

	_backupConfiguration := configured[0].(map[string]interface{})
	return &sqladmin.BackupConfiguration{
		BinaryLogEnabled: safeBool(_backupConfiguration["binary_log_enabled"]),
		Enabled:          safeBool(_backupConfiguration["enabled"]),
		StartTime:        safeString(_backupConfiguration["start_time"]),
		Location:         safeString(_backupConfiguration["location"]),
	}
}

func safeBool(v interface{}) bool {
	if v == nil {
		return false
	}
	return v.(bool)
}

func safeInt(v interface{}) int {
	if v == nil {
		return 0
	}
	return v.(int)
}

func safeString(v interface{}) string {
	if v == nil {
		return ""
	}
	return v.(string)
}
