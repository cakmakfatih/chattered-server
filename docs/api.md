# Chattered API

This document defines the HTTP API contract between the Chattered mobile app and Go backend. It is intended for mobile developers, backend developers, and coding agents. Update it whenever an endpoint, request, response, or validation rule changes.

## Contents

- [Conventions](#conventions)
- [Authentication](#authentication)
- [Endpoints](#endpoints)
- [Common errors](#common-errors)
- [Onboarding flow](#onboarding-flow)

## Conventions

- The API prefix is `/api/v1`.
- Request and response bodies use JSON objects.
- Send `Content-Type: application/json` when sending a request body.
- Protected routes require a Clerk session token in the `Authorization` header.
- Top-level request fields not listed for an endpoint are rejected.
- Request bodies are limited to 16 KiB.
- Error responses use the same JSON envelope described in [Common errors](#common-errors).

### CORS

The API allows any origin (`*`), allows the `Authorization` and `Content-Type` headers, and lists `GET`, `POST`, and `OPTIONS` as allowed methods. CORS preflight `OPTIONS` requests return `204 No Content` before authentication runs.

## Authentication

The mobile app handles Clerk sign-up and sign-in. It sends the resulting Clerk session token to this API as a bearer token:

```http
Authorization: Bearer <clerk-session-token>
```

The API verifies the token. A valid session must contain both a Clerk user ID (`sub`) and a session ID. The user ID from the verified token is used for profile lookup and creation; clients must not provide their own Clerk user ID.

Missing, malformed, or invalid credentials return `401 Unauthorized`. The response does not include token-verification details.

## Endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/me` | Read the authenticated user's local registration status and profile. |
| `POST` | `/api/v1/onboarding/username/check` | Validate username format and check current availability. |
| `POST` | `/api/v1/onboarding/bio/validate` | Validate and normalize optional biography text. |
| `POST` | `/api/v1/onboarding/photo/validate` | Validate optional profile photo metadata before upload. |
| `POST` | `/api/v1/onboarding/photo/upload` | Get a short-lived direct-to-Tigris upload authorization. |
| `POST` | `/api/v1/onboarding/complete` | Verify submitted profile data and finish registration. |

Every route in this table requires a verified Clerk session. The only exception is a CORS `OPTIONS` preflight, which is handled before authentication. JSON endpoints accept one JSON object and reject unknown top-level fields. The direct file transfer uses the Tigris signed URL and does not pass image bytes through the Chattered API.

### Get the authenticated user's registration status

```http
GET /api/v1/me
```

Returns whether the authenticated Clerk user has a Chattered profile. This endpoint does not create a profile.

**Request:** No body.

**Responses:**

- `200 OK` when the profile lookup succeeds.
- `401 Unauthorized` when the Clerk session is missing or invalid.
- `500 Internal Server Error` when the profile cannot be loaded.

Incomplete registration:

```json
{
  "registration_complete": false,
  "user": null
}
```

Completed registration:

```json
{
  "registration_complete": true,
  "user": {
    "username": "alice_1",
    "bio": "Hello from Chattered",
    "gender": "female"
  }
}
```

`bio` is `null` when the user has no biography. The response does not expose the Clerk user ID or profile image storage key.

This endpoint does not return a profile photo URL. It reports `registration_complete: false` until the profile record is created. An upload authorization or a successful Tigris PUT by itself does not change this response.

### Check username format and availability

```http
POST /api/v1/onboarding/username/check
```

Checks the username format and whether it is already used. This is a preliminary availability check only; it does not reserve the username. Registration checks again when the profile is created.

**Request body:**

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `username` | string | Yes | Candidate username. |

Example:

```json
{
  "username": "alice_1"
}
```

Username rules:

- 3–15 characters.
- Must start with a lowercase ASCII letter (`a`–`z`).
- Remaining characters may be lowercase ASCII letters, digits, or underscores.
- Uppercase letters, spaces, hyphens, and non-ASCII letters are not accepted.

**Responses:**

- `200 OK` with `available: true` or `available: false` for a valid username.
- `401 Unauthorized` when the Clerk session is missing or invalid.
- `400 Bad Request` when the body is not one JSON object.
- `422 Unprocessable Entity` when the username is invalid or an unexpected top-level field is present.
- `500 Internal Server Error` when availability cannot be checked.

Errors use the common error envelope. Invalid username errors have code `invalid_username` and field `username`; an unexpected field has code `unexpected_field` and identifies that field.

Available username:

```json
{
  "available": true
}
```

An already-used username returns `200 OK` with `available: false`; it is not an API error.

### Validate a biography

```http
POST /api/v1/onboarding/bio/validate
```

Validates an optional biography without saving it. An omitted, `null`, or empty `bio` is normalized to `null`.

**Request body:**

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `bio` | string or `null` | No | Biography to validate. An empty string is treated as absent. |

Example:

```json
{
  "bio": "Hello!\nCoffee, books & code."
}
```

Biography rules:

- At most 300 Unicode code points.
- Newlines and tabs are allowed.
- Other Unicode control characters are rejected.
- Punctuation and ordinary whitespace are allowed.

**Responses:**

- `200 OK` when the biography is valid.
- `401 Unauthorized` when the Clerk session is missing or invalid.
- `400 Bad Request` when the body is not one JSON object.
- `422 Unprocessable Entity` when the value has the wrong type, violates a biography rule, or includes an unexpected top-level field.

Errors use the common error envelope. Invalid biography errors have code `invalid_bio` and field `bio`; an unexpected field has code `unexpected_field`.

Valid biography:

```json
{
  "valid": true,
  "normalized_bio": "Hello!\nCoffee, books & code."
}
```

Absent biography:

```json
{
  "valid": true,
  "normalized_bio": null
}
```

### Validate profile photo metadata

```http
POST /api/v1/onboarding/photo/validate
```

Checks photo metadata only. It does not receive image bytes or upload a file. A successful response is preliminary; upload authorization and registration completion validate the metadata again.

**Request body:**

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `photo` | object or `null` | No | Metadata for the selected photo. Omitted or `null` means no photo was selected. |

When `photo` is an object, it must contain:

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `file_name` | string | Yes | File name without a directory path. |
| `mime_type` | string | Yes | MIME type that matches the extension. |
| `size_bytes` | integer | Yes | File size from 1 byte through 10 MiB inclusive. |

Supported extensions and MIME types:

| Extension | MIME type |
| --- | --- |
| `.jpg`, `.jpeg` | `image/jpeg` |
| `.png` | `image/png` |
| `.heic` | `image/heic` |

Extensions are case-insensitive. The MIME type must match exactly. Directory separators (`/` or `\\`) in `file_name` are rejected. The maximum size is 10 MiB (`10,485,760` bytes).

Example:

```json
{
  "photo": {
    "file_name": "avatar.jpg",
    "mime_type": "image/jpeg",
    "size_bytes": 1024
  }
}
```

**Responses:**

- `200 OK` with `valid: true` when metadata is valid or no photo was selected.
- `401 Unauthorized` when the Clerk session is missing or invalid.
- `400 Bad Request` when the body is not one JSON object.
- `422 Unprocessable Entity` when metadata is malformed, unsupported, too large, or an unexpected top-level field is present.

Invalid metadata errors use code `invalid_photo` and identify the invalid metadata field, such as `photo.file_name`, `photo.mime_type`, or `photo.size_bytes`. An unexpected top-level field uses code `unexpected_field`. This endpoint does not reserve a key, create a Tigris object, or persist metadata.

Valid metadata or no photo:

```json
{
  "valid": true
}
```

### Authorize a direct profile photo upload

```http
POST /api/v1/onboarding/photo/upload
```

Validates the selected photo metadata and returns a short-lived, server-signed URL for uploading directly to the configured private Tigris bucket. The client sends the file bytes with an HTTP `PUT` to `upload_url` and includes the returned `Content-Type` and `Content-Length` headers exactly. The signature binds the request to the server-selected bucket, object key, content type, and declared length. The object key is derived from the verified Clerk identity and has the form `users/{clerk_user_id}/profile/avatar`; filenames from the client are never part of the key. Requests and retries for the same authenticated user reuse that object location. The authorization expires after 10 minutes.

**Request body:**

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `photo` | object | Yes | Photo metadata satisfying the rules above. |

**Successful response (`200 OK`):**

```json
{
  "upload_url": "https://t3.storage.dev/example-signed-url",
  "method": "PUT",
  "expires_at": "2026-10-04T12:00:00Z",
  "headers": {
    "Content-Type": "image/jpeg",
    "Content-Length": "1024"
  }
}
```

The signed URL is a bearer credential. Keep it private, do not log it, and send no Clerk `Authorization` token to Tigris. Upload the original file bytes using the returned method and headers. The Chattered server does not receive upload progress; the mobile client can observe progress on its PUT request and should stop its loading state on either success or error.

If authorization fails, the API returns `401 Unauthorized` for an invalid session, `400 Bad Request` for a malformed JSON body, `409 Conflict` with `profile_already_complete` when the user already has a profile, `422 Unprocessable Entity` with `invalid_photo` or `unexpected_field` for invalid input, or `500 Internal Server Error` if the profile lookup or storage signing operation fails. Storage credentials and provider diagnostics are not included in errors.

The authorization request does not reserve a registration record and does not mark registration complete. If the app closes during upload, `GET /api/v1/me` remains incomplete. When the app can retry, request a fresh authorization; it targets the same object key. A previously issued URL may also remain usable until its expiration, so treat it as secret and do not share it.

The PUT request goes to Tigris, not to `/api/v1`. Its success response is provided by Tigris. Only after the PUT succeeds should the mobile app call `POST /api/v1/onboarding/complete`. For browser-based clients, the configured Tigris bucket must also allow the app origin and `PUT`/the signed headers in its CORS policy; the API's own CORS policy does not configure Tigris.

### Complete registration

```http
POST /api/v1/onboarding/complete
```

Validates all onboarding fields again and creates the Chattered profile for the authenticated Clerk user. The username availability check is repeated here because a name can be taken after the preliminary check. A database unique-constraint race is also returned as a conflict.

**Request body:**

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `username` | string | Yes | Username satisfying the rules above. |
| `gender` | string | Yes | One of `male`, `female`, or `other`. |
| `bio` | string or `null` | No | Biography satisfying the rules above. Empty string and `null` are stored as absent. |
| `photo` | object or `null` | No | Optional photo metadata satisfying the rules above. When present, the matching object must already exist in Tigris and pass server-side verification before the profile is created. |

When `photo` is omitted or `null`, registration completes without a profile photo. When metadata is present, the server reads the server-owned object key from Tigris and verifies the object's existence, stored MIME type, stored and actual byte length, and supported image format before it creates the profile. JPEG and PNG images are also checked against the pixel-dimension limit. The submitted filename is used only to confirm the expected format; it does not select the Tigris key.

Example:

```json
{
  "username": "alice_1",
  "gender": "female",
  "bio": "Hello from Chattered",
  "photo": {
    "file_name": "avatar.jpg",
    "mime_type": "image/jpeg",
    "size_bytes": 1024
  }
}
```

Do not include a `clerk_user_id` field. The identity is taken from the verified session, and unknown top-level fields are rejected.

**Responses:**

- `201 Created` when the profile is created.
- `200 OK` when an identical completion request is retried after its earlier response was lost.
- `401 Unauthorized` when the Clerk session is missing or invalid.
- `400 Bad Request` when the body is not one JSON object.
- `409 Conflict` when the profile already exists or the username is already in use.
- `409 Conflict` with `photo_upload_incomplete` when metadata is supplied but the object has not been uploaded.
- `409 Conflict` with `profile_already_complete` when another profile exists and the submitted profile fields do not match it.
- `422 Unprocessable Entity` when any field is invalid, the stored object's MIME type, size, format, or dimensions fail verification, or an unexpected top-level field is present.
- `500 Internal Server Error` when a repository or storage operation fails for another reason.

A missing uploaded object returns `photo_upload_incomplete` on field `photo`; the mobile app can let the user retry the upload. A stored-object mismatch returns `invalid_photo`. A failed storage read returns `internal_error` without exposing Tigris details. If the server created the profile but the response was lost, resubmitting matching profile fields returns `200 OK` with the completed profile instead of creating another record. Conflicting profile fields return `409 Conflict`.

Created profile:

```json
{
  "registration_complete": true,
  "user": {
    "username": "alice_1",
    "bio": "Hello from Chattered",
    "gender": "female"
  }
}
```

## Common errors

All API errors use this envelope. `field` and `details` are included only when relevant.

```json
{
  "error": {
    "code": "invalid_username",
    "message": "Username must be 3-15 characters, start with a lowercase letter, and use only lowercase letters, numbers, or underscores",
    "field": "username"
  }
}
```

| HTTP status | Error code | Meaning |
| --- | --- | --- |
| `400 Bad Request` | `invalid_request` | The body is not one JSON object. |
| `401 Unauthorized` | `unauthorized` | A valid Clerk session is required. |
| `409 Conflict` | `profile_already_complete` | A local profile already exists for this Clerk user. |
| `409 Conflict` | `photo_upload_incomplete` | The requested profile photo is not available in Tigris. |
| `409 Conflict` | `username_taken` | The username is already in use. |
| `422 Unprocessable Entity` | `unexpected_field` | An unlisted top-level request field was supplied. |
| `422 Unprocessable Entity` | `invalid_username` | Username is missing, has the wrong type, or violates its format. |
| `422 Unprocessable Entity` | `invalid_gender` | Gender is missing, has the wrong type, or is not an accepted value. |
| `422 Unprocessable Entity` | `invalid_bio` | Biography has the wrong type or violates a biography rule. |
| `422 Unprocessable Entity` | `invalid_photo` | Photo metadata or uploaded object content is malformed or violates a photo rule. |
| `500 Internal Server Error` | `internal_error` | A server-side dependency or repository operation failed. |

For an oversized photo, the error includes the limit in `details`:

```json
{
  "error": {
    "code": "invalid_photo",
    "message": "Photo size must be between 1 byte and 10 MiB",
    "field": "photo.size_bytes",
    "details": {
      "max_size_bytes": 10485760
    }
  }
}
```

Unexpected fields are checked at the top level. Error messages are in English; clients should use `code` and `field` for programmatic handling rather than parsing `message`.

## Onboarding flow

1. The mobile app authenticates the user with Clerk and keeps the resulting session token.
2. Call `GET /api/v1/me` to check whether the authenticated Clerk user already has a Chattered profile.
3. For an incomplete profile, call `POST /api/v1/onboarding/username/check` as the user enters a username. A positive availability result does not reserve the name.
4. Call `POST /api/v1/onboarding/bio/validate` and `POST /api/v1/onboarding/photo/validate` to give early feedback. These endpoints do not persist data.
5. If a photo was selected, request `POST /api/v1/onboarding/photo/upload` with the metadata. Keep the returned signed URL private.
6. PUT the photo bytes directly to the returned URL with the returned `Content-Type` and `Content-Length` headers. Do not send the Clerk token to Tigris. Wait for the PUT to succeed or show an upload error and allow retry.
7. Submit all profile fields, including the same photo metadata, to `POST /api/v1/onboarding/complete`. The server repeats every validation, verifies the stored object, uses the verified Clerk user ID, checks current username availability, and creates the profile.

If the app closes before completion, `GET /api/v1/me` continues to report registration as incomplete. The app should leave onboarding in a recoverable state rather than showing an endless loading state. When the user resumes, it can request fresh authorization and upload again, then retry completion. Completion retries are idempotent when the saved username, gender, and biography match and the photo-presence state matches. A successful `/me` response does not include the profile image URL or storage key; a profile-photo retrieval route is not currently part of this API.
