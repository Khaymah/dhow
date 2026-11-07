package i18n

import "strings"

type docsRef struct {
	Page     string
	ArAnchor string
	EnAnchor string
}

const (
	pageTroubleshooting = "install/troubleshooting.md"
	pageValues          = "guides/secrets.md"
	pageIngest          = "guides/ingest.md"
	pageEnvironments    = "guides/environments.md"
	pageDeploySSH       = "guides/deploy-ssh.md"
	pageOidc            = "guides/oidc.md"
	pageGit             = "guides/git.md"
	pageOrganization    = "guides/organization.md"
	pageSettings        = "guides/instance-settings.md"
	pageJobs            = "guides/jobs.md"
	pageProjects        = "guides/projects.md"
	pageRunnerRegister  = "runner/register.md"
	pageNotifications   = "guides/notifications.md"
)

var (
	refCsrf           = docsRef{pageTroubleshooting, "يفشل-الإعداد-أو-تسجيل-الدخول-بـ-csrf_failed", "setup-or-sign-in-fails-with-csrf_failed"}
	refRateLimit      = docsRef{pageTroubleshooting, "429-rate_limited-في-صفحات-الدخول-أو-الدعوات", "429-rate_limited-on-sign-in-pages-or-invitations"}
	refSetupLink      = docsRef{pageTroubleshooting, "العثور-على-رابط-الإعداد", "finding-the-setup-link"}
	refNotStarting    = docsRef{pageTroubleshooting, "داو-لا-يبدأ", "dhow-does-not-start"}
	refSecretWrite    = docsRef{pageValues, "كتابة-سر", "write-a-secret"}
	refSecretStore    = docsRef{pageValues, "أين-تحفظ-السر", "where-a-secret-lives"}
	refSecretVersions = docsRef{pageValues, "الإصدارات", "versions"}
	refVariables      = docsRef{pageValues, "المتغيّرات", "variables"}
	refRegistry       = docsRef{pageValues, "حسابات-سجلات-الصور", "registry-credentials"}
	refIngestToken    = docsRef{pageIngest, "التوكن", "the-token"}
	refIngestUpload   = docsRef{pageIngest, "الرفع-بـ-curl", "upload-with-curl"}
	refIngestSize     = docsRef{pageIngest, "الوكيل-العكسي-والحجم", "reverse-proxy-and-size"}
	refEnvCreate      = docsRef{pageEnvironments, "إنشاء-بيئة", "create-an-environment"}
	refEnvRules       = docsRef{pageEnvironments, "تغيير-القواعد", "change-the-rules"}
	refEnvApprovals   = docsRef{pageEnvironments, "الموافقات", "approvals"}
	refEnvBypass      = docsRef{pageEnvironments, "تخطّي-المراجعين", "bypass-the-reviewers"}
	refEnvPause       = docsRef{pageEnvironments, "الإيقاف-وإعادة-التشغيل", "pause-and-resume"}
	refTargets        = docsRef{pageEnvironments, "أهداف-النشر", "deploy-targets"}
	refAccessKey      = docsRef{pageEnvironments, "مفتاح-الدخول", "the-access-key"}
	refPinKey         = docsRef{pageEnvironments, "تثبيت-مفتاح-السيرفر", "pin-the-key-of-a-server"}
	refTargetCheck    = docsRef{pageDeploySSH, "فحص-الهدف", "checking-a-target"}
	refRollback       = docsRef{pageDeploySSH, "الرجوع-إلى-إصدار-سابق", "rolling-back-to-an-earlier-release"}
	refOidcTrouble    = docsRef{pageOidc, "حل-المشكلات", "troubleshooting"}
	refOidcLink       = docsRef{pageOidc, "4-ربط-حساب", "4-link-an-account"}
	refOidcConfigure  = docsRef{pageOidc, "3-ضبط-داو", "3-configure-dhow"}
	refProviders      = docsRef{pageSettings, "مزودو-الدخول", "sign-in-providers"}
	refLocked         = docsRef{pageSettings, "الإعدادات-المقفلة-وdoctor", "locked-settings-and-doctor"}
	refSaving         = docsRef{pageSettings, "الحفظ", "saving"}
	refMail           = docsRef{pageSettings, "تجربة-البريد", "testing-mail"}
	refGitHostKey     = docsRef{pageGit, "مفتاح-سيرفر-المستودع", "the-host-key"}
	refGitDeployKey   = docsRef{pageGit, "مفتاح-المستودع", "the-deploy-key"}
	refProjectLink    = docsRef{pageProjects, "ربط-مستودع", "link-a-repository"}
	refProjectRuns    = docsRef{pageProjects, "التنفيذات", "runs"}
	refProjectManual  = docsRef{pageProjects, "التنفيذ-اليدوي", "manual-runs"}
	refOrgRoles       = docsRef{pageOrganization, "من-يستطيع-ماذا", "who-can-do-what"}
	refOrgAccount     = docsRef{pageOrganization, "حسابك", "your-account"}
	refJobPage        = docsRef{pageJobs, "صفحة-التشغيل", "a-job"}
	refJobCreate      = docsRef{pageJobs, "إنشاء-تشغيل-عبر-api", "create-a-job-through-the-api"}
	refChannels       = docsRef{pageNotifications, "القنوات", "channels"}
	refRefused        = docsRef{pageNotifications, "ما-يرفض-داو-الوصول-إليه", "what-dhow-refuses-to-reach"}
	refDelivery       = docsRef{pageNotifications, "التسليم-وإعادة-المحاولة", "delivery-and-retries"}
	refNotifyTrouble  = docsRef{pageNotifications, "استكشاف-الأخطاء", "troubleshooting"}
)

var docsRefs = map[string]docsRef{
	"CHANNEL_EXISTS":                       refChannels,
	"CHANNEL_HOST_NOT_ALLOWED":             refRefused,
	"CHANNEL_HOST_BLOCKED":                 refRefused,
	"CHANNEL_SECRET_INVALID":               refChannels,
	"CHANNEL_VERSION_CONFLICT":             refChannels,
	"CHANNEL_DISABLED":                     refChannels,
	"DELIVERY_NOT_RETRYABLE":               refDelivery,
	"TEST_RATE_LIMITED":                    refNotifyTrouble,
	"SERVICE_UNAVAILABLE":                  refNotStarting,
	"FORBIDDEN":                            refOrgRoles,
	"CSRF_FAILED":                          refCsrf,
	"PASSWORD_BREACHED":                    refOrgAccount,
	"SETUP_TOKEN_INVALID":                  refSetupLink,
	"INVALID_MFA_CODE":                     refOrgAccount,
	"INVALID_CURRENT_FACTOR":               refOrgAccount,
	"ENROLLMENT_EXPIRED":                   refOrgAccount,
	"PASSKEY_REJECTED":                     refOrgAccount,
	"RUNNER_REVOKED":                       {Page: pageRunnerRegister},
	"SECRETS_UNAVAILABLE":                  refSecretStore,
	"JOB_FINISHED":                         refJobPage,
	"JOB_QUEUE_FULL":                       refJobCreate,
	"ATTEMPT_NOT_FOUND":                    refJobPage,
	"TOO_MANY_STREAMS":                     refJobPage,
	"RATE_LIMITED":                         refRateLimit,
	"LAST_OWNER":                           refOrgRoles,
	"ROLE_NOT_ALLOWED":                     refOrgRoles,
	"OIDC_FLOW_INVALID":                    refOidcTrouble,
	"OIDC_SIGN_IN_FAILED":                  refOidcTrouble,
	"OIDC_NOT_LINKED":                      refOidcLink,
	"OIDC_DOMAIN_NOT_ALLOWED":              refOidcConfigure,
	"OIDC_PROVIDER_UNAVAILABLE":            refOidcTrouble,
	"OIDC_IDENTITY_IN_USE":                 refOidcLink,
	"OIDC_ALREADY_LINKED":                  refOidcLink,
	"LAST_SIGN_IN_METHOD":                  refOrgAccount,
	"SECOND_FACTOR_REQUIRED":               refOrgAccount,
	"PROJECT_ARCHIVED":                     {Page: pageProjects},
	"FORK_POLICY_NOT_ALLOWED":              {Page: pageProjects},
	"FORK_CACHE_NOT_ALLOWED":               {Page: pageProjects},
	"ENVIRONMENT_EXISTS":                   refEnvCreate,
	"ENVIRONMENT_VERSION_CONFLICT":         refEnvRules,
	"REVIEWER_NOT_ELIGIBLE":                refEnvApprovals,
	"SELF_REVIEW_NOT_ALLOWED":              refEnvApprovals,
	"REVIEWER_NEEDS_SECOND_FACTOR":         refEnvApprovals,
	"APPROVAL_NOT_PENDING":                 refEnvApprovals,
	"BYPASS_NOT_ALLOWED":                   refEnvBypass,
	"ARTIFACT_GONE":                        {Page: pageIngest},
	"INGEST_TOKEN_INVALID":                 refIngestToken,
	"INGEST_TOKEN_REVOKED":                 refIngestToken,
	"INGEST_TOKEN_EXPIRED":                 refIngestToken,
	"INGEST_TOKEN_LIMIT":                   refIngestToken,
	"INGEST_TOKEN_NAME_TAKEN":              refIngestToken,
	"REF_NOT_ALLOWED_FOR_TOKEN":            refIngestToken,
	"DIGEST_MISMATCH":                      refIngestUpload,
	"ARCHIVE_UNSAFE":                       refIngestUpload,
	"ARTIFACT_TOO_LARGE":                   refIngestSize,
	"ARTIFACT_QUOTA_EXCEEDED":              refIngestSize,
	"STORAGE_FULL":                         refIngestSize,
	"UPLOAD_TOO_SLOW":                      refIngestSize,
	"INGEST_TOKEN_QUOTA":                   refIngestToken,
	"INGEST_TOKEN_REACH_UNCONFIRMED":       refIngestToken,
	"INGEST_CREATOR_NOT_ALLOWED":           refIngestToken,
	"INGEST_REACH_NOT_ACKNOWLEDGED":        refIngestToken,
	"INGEST_REACH_NEEDS_ENVIRONMENT_ADMIN": refIngestToken,
	"INGEST_REACH_CHANGED":                 refIngestToken,
	"SECRET_TOO_SHORT":                     refSecretWrite,
	"SECRET_TOO_LARGE":                     refSecretWrite,
	"SECRET_NAME_RESERVED":                 refSecretWrite,
	"SECRET_LIMIT_REACHED":                 refSecretWrite,
	"SECRET_VERSION_ERASED":                refSecretVersions,
	"VARIABLE_LIMIT_REACHED":               refVariables,
	"REGISTRY_CREDENTIAL_EXISTS":           refRegistry,
	"REGISTRY_CREDENTIAL_LIMIT_REACHED":    refRegistry,
	"SETTING_LOCKED_BY_ENV":                refLocked,
	"SETTING_CHANGED":                      refSaving,
	"SETTING_INVALID":                      refSaving,
	"MAIL_NOT_CONFIGURED":                  refMail,
	"MAIL_TEST_FAILED":                     refMail,
	"OIDC_PROVIDER_EXISTS":                 refProviders,
	"OIDC_PROVIDER_LIMIT":                  refProviders,
	"OIDC_PROVIDER_LINKED":                 refProviders,
	"OIDC_PROVIDER_ONLY_SIGN_IN":           refProviders,
	"REPOSITORY_NOT_LINKED":                refProjectLink,
	"GIT_HOST_BLOCKED":                     refGitHostKey,
	"GIT_HOST_UNREACHABLE":                 refGitHostKey,
	"HOST_KEY_NOT_PENDING":                 refGitHostKey,
	"HOST_KEY_FINGERPRINT_MISMATCH":        refGitHostKey,
	"DEPLOY_KEY_NOT_APPLICABLE":            refGitDeployKey,
	"RUN_NOT_RERUNNABLE":                   refProjectRuns,
	"RUN_NOT_AWAITING_APPROVAL":            refProjectRuns,
	"SHA_NOT_ON_REF":                       refProjectManual,
	"REF_HEAD_UNKNOWN":                     refProjectManual,
	"TARGET_EXISTS":                        refTargets,
	"TARGET_DUPLICATE":                     refTargets,
	"TARGET_VERSION_CONFLICT":              refTargets,
	"TARGET_HOST_BLOCKED":                  refPinKey,
	"TARGET_HOST_UNREACHABLE":              refPinKey,
	"TARGET_KEY_NOT_FOUND":                 refAccessKey,
	"DEPLOY_PATH_INVALID":                  refTargets,
	"TARGET_KEY_NAME_TAKEN":                refAccessKey,
	"TARGET_CHECK_UNAVAILABLE":             refTargetCheck,
	"CHECK_RUN_NOT_RERUNNABLE":             refTargetCheck,
	"ROLLBACK_RUN_NOT_RERUNNABLE":          refRollback,
	"ROLLBACK_RELEASE_NOT_ELIGIBLE":        refRollback,
	"ROLLBACK_UNAVAILABLE":                 refRollback,
	"ROLLBACK_NOT_SUPPORTED":               refRollback,
	"ROLLBACK_SETTINGS_INVALID":            refRollback,
	"ENVIRONMENT_PAUSED":                   refEnvPause,
	"TARGET_KEY_NEEDS_FACTOR":              refAccessKey,
}

func Fix(locale Locale, code string) string {
	if locale == En {
		return enFixes[code]
	}
	return arFixes[code]
}

func FixCodes(locale Locale) []string {
	m := arFixes
	if locale == En {
		m = enFixes
	}
	out := make([]string, 0, len(m))
	for code := range m {
		out = append(out, code)
	}
	return out
}

func DocsPath(locale Locale, code string) string {
	ref, ok := docsRefs[code]
	if !ok {
		return ""
	}
	anchor := ref.ArAnchor
	if locale == En {
		anchor = ref.EnAnchor
	}
	if anchor == "" {
		return ref.Page
	}
	return ref.Page + "#" + anchor
}

func DocsCodes() []string {
	out := make([]string, 0, len(docsRefs))
	for code := range docsRefs {
		out = append(out, code)
	}
	return out
}

func SplitDocsPath(path string) (string, string) {
	page, anchor, _ := strings.Cut(path, "#")
	return page, anchor
}
