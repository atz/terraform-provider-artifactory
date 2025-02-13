package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	datasource_artifact "github.com/jfrog/terraform-provider-artifactory/v11/pkg/artifactory/datasource/artifact"
	datasource_repository "github.com/jfrog/terraform-provider-artifactory/v11/pkg/artifactory/datasource/repository"
	rs "github.com/jfrog/terraform-provider-artifactory/v11/pkg/artifactory/resource"
	"github.com/jfrog/terraform-provider-artifactory/v11/pkg/artifactory/resource/configuration"
	"github.com/jfrog/terraform-provider-artifactory/v11/pkg/artifactory/resource/security"
	"github.com/jfrog/terraform-provider-artifactory/v11/pkg/artifactory/resource/user"
	"github.com/jfrog/terraform-provider-shared/client"
	"github.com/jfrog/terraform-provider-shared/util"
	validatorfw_string "github.com/jfrog/terraform-provider-shared/validator/fw/string"
)

// Ensure the implementation satisfies the provider.Provider interface.
var _ provider.Provider = &ArtifactoryProvider{}

type ArtifactoryProvider struct{}

// ArtifactoryProviderModel describes the provider data model.
type ArtifactoryProviderModel struct {
	Url              types.String `tfsdk:"url"`
	AccessToken      types.String `tfsdk:"access_token"`
	ApiKey           types.String `tfsdk:"api_key"`
	OIDCProviderName types.String `tfsdk:"oidc_provider_name"`
	CheckLicense     types.Bool   `tfsdk:"check_license"`
}

// Metadata satisfies the provider.Provider interface for ArtifactoryProvider
func (p *ArtifactoryProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "artifactory"
	resp.Version = Version
}

// Schema satisfies the provider.Provider interface for ArtifactoryProvider.
func (p *ArtifactoryProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Description: "Artifactory URL.",
				Optional:    true,
				Validators: []validator.String{
					validatorfw_string.IsURLHttpOrHttps(),
				},
			},
			"access_token": schema.StringAttribute{
				Description: "This is a access token that can be given to you by your admin under `User Management -> Access Tokens`. If not set, the 'api_key' attribute value will be used.",
				Optional:    true,
				Sensitive:   true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"api_key": schema.StringAttribute{
				Description:        "API key. If `access_token` attribute, `JFROG_ACCESS_TOKEN` or `ARTIFACTORY_ACCESS_TOKEN` environment variable is set, the provider will ignore this attribute.",
				DeprecationMessage: "An upcoming version will support the option to block the usage/creation of API Keys (for admins to set on their platform).\nIn a future version (scheduled for end of Q3, 2023), the option to disable the usage/creation of API Keys will be available and set to disabled by default. Admins will be able to enable the usage/creation of API Keys.\nBy end of Q4 2024, API Keys will be deprecated all together and the option to use them will no longer be available. See [JFrog API deprecation process](https://jfrog.com/help/r/jfrog-platform-administration-documentation/jfrog-api-key-deprecation-process) for more details.",
				Optional:           true,
				Sensitive:          true,
			},
			"oidc_provider_name": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				Description: "OIDC provider name. See [Configure an OIDC Integration](https://jfrog.com/help/r/jfrog-platform-administration-documentation/configure-an-oidc-integration) for more details.",
			},
			"check_license": schema.BoolAttribute{
				Description: "Toggle for pre-flight checking of Artifactory Pro and Enterprise license. Default to `true`.",
				Optional:    true,
			},
		},
	}
}

func (p *ArtifactoryProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// Check environment variables, first available OS variable will be assigned to the var
	url := util.CheckEnvVars([]string{"JFROG_URL", "ARTIFACTORY_URL"}, "")
	accessToken := util.CheckEnvVars([]string{"JFROG_ACCESS_TOKEN", "ARTIFACTORY_ACCESS_TOKEN"}, "")

	var config ArtifactoryProviderModel

	// Read configuration data into model
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if config.Url.ValueString() != "" {
		url = config.Url.ValueString()
	}

	if url == "" {
		resp.Diagnostics.AddError(
			"Missing URL Configuration",
			"While configuring the provider, the url was not found in "+
				"the JFROG_URL/ARTIFACTORY_URL environment variable or provider "+
				"configuration block url attribute.",
		)
		return
	}

	restyClient, err := client.Build(url, productId)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating Resty client",
			err.Error(),
		)
		return
	}

	oidcAccessToken, err := util.OIDCTokenExchange(ctx, restyClient, config.OIDCProviderName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed OIDC ID token exchange",
			err.Error(),
		)
		return
	}

	// use token from OIDC provider, which should take precedence over
	// environment variable data, if found.
	if oidcAccessToken != "" {
		accessToken = oidcAccessToken
	}

	// Check configuration data, which should take precedence over
	// environment variable data, if found.
	if config.AccessToken.ValueString() != "" {
		accessToken = config.AccessToken.ValueString()
	}

	apiKey := config.ApiKey.ValueString()

	if apiKey == "" && accessToken == "" {
		resp.Diagnostics.AddError(
			"Missing JFrog API key or Access Token",
			"While configuring the provider, the API key or Access Token was not found in "+
				"the environment variables or provider configuration attributes.",
		)
		return
	}

	restyClient, err = client.AddAuth(restyClient, apiKey, accessToken)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error adding Auth to Resty client",
			err.Error(),
		)
	}

	if config.CheckLicense.IsNull() || config.CheckLicense.ValueBool() {
		if err := util.CheckArtifactoryLicense(restyClient, "Enterprise", "Commercial", "Edge"); err != nil {
			resp.Diagnostics.AddError(
				"Error checking Artifactory license",
				err.Error(),
			)
			return
		}
	}

	version, err := util.GetArtifactoryVersion(restyClient)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error getting Artifactory version",
			fmt.Sprintf("The provider functionality might be affected by the absence of Artifactory version in the context. %v", err),
		)
		return
	}

	featureUsage := fmt.Sprintf("Terraform/%s", req.TerraformVersion)
	go util.SendUsage(ctx, restyClient.R(), productId, featureUsage)

	meta := util.ProviderMetadata{
		Client:             restyClient,
		ProductId:          productId,
		ArtifactoryVersion: version,
	}

	resp.DataSourceData = meta
	resp.ResourceData = meta
}

// Resources satisfies the provider.Provider interface for ArtifactoryProvider.
func (p *ArtifactoryProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		rs.NewArtifactResource,
		user.NewAnonymousUserResource,
		user.NewManagedUserResource,
		user.NewUnmanagedUserResource,
		user.NewUserResource,
		security.NewGroupResource,
		security.NewScopedTokenResource,
		security.NewGlobalEnvironmentResource,
		security.NewDistributionPublicKeyResource,
		security.NewCertificateResource,
		security.NewKeyPairResource,
		security.NewPasswordExpirationPolicyResource,
		security.NewUserLockPolicyResource,
		configuration.NewLdapSettingResource,
		configuration.NewLdapGroupSettingResource,
		configuration.NewBackupResource,
		configuration.NewGeneralSecurityResource,
		configuration.NewMailServerResource,
		configuration.NewPropertySetResource,
		configuration.NewProxyResource,
		configuration.NewRepositoryLayoutResource,
	}
}

// DataSources satisfies the provider.Provider interface for ArtifactoryProvider.
func (p *ArtifactoryProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		datasource_repository.NewRepositoriesDataSource,
		datasource_artifact.NewFileListDataSource,
	}
}

func Framework() func() provider.Provider {
	return func() provider.Provider {
		return &ArtifactoryProvider{}
	}
}
