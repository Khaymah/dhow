package sandbox

const (
	OutputPath = "/tmp/.dhow-output"

	CheckoutCommand        = "__checkout"
	CheckoutEnvURL         = "DHOW_CHECKOUT_URL"
	CheckoutEnvRef         = "DHOW_CHECKOUT_REF"
	CheckoutEnvSha         = "DHOW_CHECKOUT_SHA"
	CheckoutEnvDepth       = "DHOW_CHECKOUT_DEPTH"
	CheckoutEnvCredential  = "DHOW_CHECKOUT_CREDENTIAL" //nolint:gosec // The name of a flag variable, not a credential.
	CheckoutCredentialFile = "git-credential"           //nolint:gosec // The name of the secret file, not a credential.
	CheckoutKnownHostsFile = "known_hosts"

	CheckoutExitOK              = 0
	CheckoutExitFailed          = 1
	CheckoutExitRefMoved        = 3
	CheckoutExitHostKeyMismatch = 4
	CheckoutExitDiskFull        = 5
)
