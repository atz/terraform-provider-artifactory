package artifactory

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var gradleLocalSchema = mergeSchema(baseLocalRepoSchema, map[string]*schema.Schema{
	"checksum_policy_type": {
		Type:     schema.TypeString,
		Optional: true,
		Default:  "client-checksums",
		Description: "Checksum policy determines how Artifactory behaves when a client checksum for a deployed " +
			"resource is missing or conflicts with the locally calculated checksum (bad checksum).\nFor more details, " +
			"please refer to Checksum Policy - " +
			"https://www.jfrog.com/confluence/display/JFROG/Local+Repositories#LocalRepositories-ChecksumPolicy",
	},
	"snapshot_version_behavior": {
		Type:     schema.TypeString,
		Optional: true,
		Default:  "unique",
		Description: "Specifies the naming convention for Maven SNAPSHOT versions.\nThe options are " +
			"-\nUnique: Version number is based on a time-stamp (default)\nNon-unique: Version number uses a" +
			" self-overriding naming pattern of artifactId-version-SNAPSHOT.type\nDeployer: Respects the settings " +
			"in the Maven client that is deploying the artifact.",
	},
	"max_unique_snapshots": {
		Type:     schema.TypeInt,
		Optional: true,
		Default:  nil, // will it work as an "empty value" ? Or just remove the default?
		Description: "The maximum number of unique snapshots of a single artifact to store.\nOnce the number of " +
			"snapshots exceeds this setting, older versions are removed.\nA value of 0 (default) indicates there is " +
			"no limit, and unique snapshots are not cleaned up.",
	},
	"handle_releases": {
		Type:        schema.TypeBool,
		Optional:    true,
		Default:     true,
		Description: "If set, Artifactory allows you to deploy release artifacts into this repository.",
	},
	"handle_snapshots": {
		Type:        schema.TypeBool,
		Optional:    true,
		Default:     true,
		Description: "If set, Artifactory allows you to deploy snapshot artifacts into this repository.",
	},
	"suppress_pom_consistency_checks": {
		Type:     schema.TypeBool,
		Optional: true,
		Default:  true,
		Description: "By default, Artifactory keeps your repositories healthy by refusing POMs with incorrect " +
			"coordinates (path).\n  If the groupId:artifactId:version information inside the POM does not match the " +
			"deployed path, Artifactory rejects the deployment with a \"409 Conflict\" error.\n  You can disable this " +
			"behavior by setting the Suppress POM Consistency Checks checkbox.",
	},
})

func resourceArtifactoryLocalGradleRepository() *schema.Resource {

	return mkResourceSchema(gradleLocalSchema, universalPack, unPackLocalGradleRepository, func() interface{} {
		return &GradleLocalRepositoryParams{
			LocalRepositoryBaseParams: LocalRepositoryBaseParams{
				PackageType: "gradle",
				Rclass:      "local",
			},
		}
	})
}

type GradleLocalRepositoryParams struct {
	LocalRepositoryBaseParams
	ChecksumPolicyType           string `hcl:"checksum_policy_type" json:"checksumPolicyType"`
	SnapshotVersionBehavior      string `hcl:"snapshot_version_behavior" json:"snapshotVersionBehavior"`
	MaxUniqueSnapshots           int    `hcl:"max_unique_snapshots" json:"maxUniqueSnapshots"`
	HandleReleases               bool   `hcl:"handle_releases" json:"handleReleases"`
	HandleSnapshots              bool   `hcl:"handle_snapshots" json:"handleSnapshots"`
	SuppressPomConsistencyChecks bool   `hcl:"suppress_pom_consistency_checks" json:"suppressPomConsistencyChecks"`
}

func unPackLocalGradleRepository(data *schema.ResourceData) (interface{}, string, error) {
	d := &ResourceData{ResourceData: data}
	repo := GradleLocalRepositoryParams{
		LocalRepositoryBaseParams:    unpackBaseLocalRepo(data, "gradle"),
		ChecksumPolicyType:           d.getString("checksum_policy_type", false),
		SnapshotVersionBehavior:      d.getString("snapshot_version_behavior", false),
		MaxUniqueSnapshots:           d.getInt("max_unique_snapshots", false),
		HandleReleases:               d.getBool("handle_releases", false),
		HandleSnapshots:              d.getBool("handle_snapshots", false),
		SuppressPomConsistencyChecks: d.getBool("suppress_pom_consistency_checks", false),
	}

	return repo, repo.Id(), nil
}
