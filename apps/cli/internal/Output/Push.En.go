package output

var enPushMessages = map[Key]string{ //nolint:gosec // Messages, not credentials.
	KeyPushUsage: `Usage:
  dhow artifact push [options] <folder | file.tar.gz>

Packs a folder, or takes a .tar.gz file you already built, and uploads it to a Dhow server.
Dhow stores the file by its SHA-256 digest and starts a run that uses exactly these bytes.

Options:
  --server URL        the Dhow server (or set DHOW_SERVER)
  --name NAME         the artifact name, as the pipeline lists it under ingested
  --ref REF           the branch or tag that was built
  --ref-kind KIND     branch (the default) or tag
  --sha COMMIT        the commit that was built
  --provenance K=V    a fact about the CI, such as Repository=Khaymah/dhow (repeat it)
  --no-trigger        store the file and do not start a run
  --token-stdin       read the ingest token from standard input
  --ca-file FILE      a PEM file of extra certificate authorities (or set DHOW_CA_FILE)
  --retries N         retries after a temporary failure, 0 to 10 (default 3)
  --dry-run           pack, then print the digest and the headers, and send nothing
  --format text|json  how to print the result
  --lang en|ar        the language of the messages

The token comes only from DHOW_INGEST_TOKEN or --token-stdin. It is never read from a flag,
because a flag shows in process lists and in logs. HTTPS_PROXY and NO_PROXY are honoured.
http:// is accepted only for localhost, so a token never crosses a network in clear text.

Exit codes:
  0  the file is stored (201), or it was already stored (200)
  1  the server refused the file or its details
  2  the command or its input is wrong, and nothing was sent
  3  the server does not accept the token, or not for this ref
  4  a temporary failure was still there after the retries
  5  the answer was not what a Dhow server sends
`,
	KeyPushPrefix:            "dhow artifact push:",
	KeyPushUnknownSubcommand: "dhow artifact: %s is not a command. Use dhow artifact push.",
	KeyPushUnknownOption:     "dhow artifact push: %s is not an option.",
	KeyPushOptionValue:       "dhow artifact push: %s needs a value.",
	KeyPushTokenFlag:         "dhow artifact push: the token is never read from a flag, because a flag shows in process lists and in logs. Set DHOW_INGEST_TOKEN, or use --token-stdin.",
	KeyPushProvenanceArg:     "dhow artifact push: --provenance needs Key=Value, for example --provenance Repository=Khaymah/dhow.",
	KeyPushRetries:           "dhow artifact push: --retries is a number from 0 to 10.",
	KeyPushNeedPath:          "dhow artifact push: give one folder or one .tar.gz file.",
	KeyPushRequired:          "dhow artifact push: %s is required (for the server, DHOW_SERVER also works).",
	KeyPushNeedToken:         "dhow artifact push: no token. Set DHOW_INGEST_TOKEN, or use --token-stdin and send the token on standard input.",
	KeyPushTokenRead:         "dhow artifact push: the token could not be read from standard input.",
	KeyPushTokenShape:        "dhow artifact push: the token is not shaped like dhow_ing_ followed by 43 characters. Check the secret you passed.",
	KeyPushLangInvalid:       "dhow artifact push: --lang is en or ar.",
	KeyPushFormatInvalid:     "dhow artifact push: --format is text or json.",
	KeyPushRetrying:          "attempt %d of %d failed: %s. Trying again in %s.",
	KeyPushRetryAfter:        "The server asks you to wait %s seconds.",
	KeyPushAttempts:          "Tried %d times.",
	KeyPushClassToken:        "The server does not accept this token for this upload.",
	KeyPushClassRefused:      "The server refused the upload.",
	KeyPushClassTemporary:    "The server could not take the upload now.",
	KeyPushClassUnexpected:   "The server answered with something that is not a Dhow reply.",
	KeyPushErrServer:         "the server must be an https:// address, or http://localhost, with no user, query or fragment.",
	KeyPushErrName:           "--name must be letters, digits, dot, underscore or hyphen, start with a letter or digit, and be at most 64 characters.",
	KeyPushErrRef:            "--ref is not a valid branch or tag name for the --ref-kind you gave.",
	KeyPushErrRefKind:        "--ref-kind is branch or tag.",
	KeyPushErrSha:            "--sha must be a full commit id in lowercase hex (40 or 64 characters), and not all zeros.",
	KeyPushErrProvenance:     "--provenance allows up to 16 fields. A key starts with a letter, uses letters, digits and underscore, and has at most 32 characters. A value is plain text of at most 256 characters.",
	KeyPushErrCAFile:         "the certificate file cannot be read, or has no certificate in PEM form.",
	KeyPushErrNotGzip:        "the file is not a .tar.gz archive.",
	KeyPushErrEmpty:          "there is nothing to send: the folder or the file is empty.",
	KeyPushErrUnsafe:         "the archive is not safe: it has a path outside the folder, a device file, or a link that leaves the folder. The server would refuse it.",
	KeyPushErrUnsupported:    "the folder has a device, a socket, or a link that leaves the folder. Remove it, or pack only what you ship.",
	KeyPushErrMissing:        "a path in the folder does not exist.",
	KeyPushErrTooLarge:       "the archive has too many entries or is too large to unpack.",
	KeyPushErrPath:           "a name in the folder is not a safe relative path.",
	KeyPushErrSource:         "the path cannot be read as a folder or a .tar.gz file (%s).",
	KeyPushDryRun:            "Dry run: nothing was sent.",
	KeyPushDigest:            "Digest: sha256:%s",
	KeyPushSize:              "Size: %d bytes",
	KeyPushStored:            "Stored %s as sha256:%s (%d bytes).",
	KeyPushDuplicate:         "Already stored: %s as sha256:%s (%d bytes). Nothing was changed.",
	KeyPushRun:               "Run #%d started: %s",
	KeyPushNoRun:             "No run was started (%s).",

	CodeKey("INGEST_TOKEN_INVALID"):      "The token is not known to this server. Check that it is the whole token, and that the server address is the right one.",
	CodeKey("INGEST_TOKEN_REVOKED"):      "The token was revoked. Make a new ingest token in the project settings.",
	CodeKey("INGEST_TOKEN_EXPIRED"):      "The token has expired. Make a new ingest token in the project settings.",
	CodeKey("REF_NOT_ALLOWED_FOR_TOKEN"): "This token may not upload for this ref. Check the ref patterns of the token.",
	CodeKey("DIGEST_MISMATCH"):           "The bytes the server received do not match their digest. The file changed or broke on the way. Run the command again.",
	CodeKey("ARCHIVE_UNSAFE"):            "The archive is not safe to unpack: it has a path outside the folder, a device file, or a link that leaves the folder.",
	CodeKey("ARTIFACT_TOO_LARGE"):        "The file is larger than this server accepts (DHOW_INGEST_MAX_BYTES).",
	CodeKey("ARTIFACT_QUOTA_EXCEEDED"):   "The organization has no room left for build artifacts. Wait for old files to expire, or ask an administrator.",
	CodeKey("LENGTH_REQUIRED"):           "The server needs the size of the file first. A proxy on the way may have changed the request.",
	CodeKey("UPLOAD_TOO_SLOW"):           "The upload was too slow, and the server stopped it.",
	CodeKey("INGEST_TOKEN_QUOTA"):        "This token has uploaded as much as it may. Revoke it and make a new one.",
	CodeKey("RATE_LIMITED"):              "Too many uploads. Wait, then run the command again.",
	CodeKey("SERVICE_UNAVAILABLE"):       "The server is busy with other uploads.",
	CodeKey("VALIDATION_FAILED"):         "The server does not accept one of the details you sent (name, ref, commit or provenance).",
	CodeKey("PROJECT_ARCHIVED"):          "The project is archived.",
	CodeKey("NETWORK"):                   "The connection to the server failed.",
	CodeKey("TIMEOUT"):                   "The upload took too long.",
	CodeKey("REDIRECT"):                  "The server sent the request somewhere else. The command never follows a redirect, so the token stays with the server you named. Use the final address in --server.",
	CodeKey("BAD_RESPONSE"):              "The answer is not a Dhow reply. Check that --server points at Dhow and not at a login page or a proxy.",
	CodeKey("RESPONSE_DIGEST_DIFFERS"):   "The server says it stored a different digest from the one sent.",
	CodeKey("HTTP_4XX"):                  "The server refused the request.",
	CodeKey("HTTP_5XX"):                  "The server failed to handle the request.",
	CodeKey("HTTP_UNEXPECTED"):           "The server sent an answer this command does not know.",
	CodeKey("HTTP_413"):                  "A proxy or the server says the file is too large. Raise the body limit for /ingest/ in the reverse proxy.",
	CodeKey("TLS"):                       "The server certificate is not trusted. If your company has its own certificate authority, pass its PEM file with --ca-file or DHOW_CA_FILE.",
}
