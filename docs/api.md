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

- 3–30 characters.
- Must start with a lowercase ASCII letter (`a`–`z`).
- Remaining characters may be lowercase ASCII letters, digits, or underscores.
- Uppercase letters, spaces, hyphens, and non-ASCII letters are not accepted.

**Responses:**

- `200 OK` with `available: true` or `available: false` for a valid username.
- `401 Unauthorized` when the Clerk session is missing or invalid.
- `400 Bad Request` when the body is not one JSON object.
- `422 Unprocessable Entity` when the username is invalid or an unexpected top-level field is present.
- `500 Internal Server Error` when availability cannot be checked.

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

- At most 750 Unicode code points.
- Newlines and tabs are allowed.
- Other Unicode control characters are rejected.
- Punctuation and ordinary whitespace are allowed.

**Responses:**

- `200 OK` when the biography is valid.
- `401 Unauthorized` when the Clerk session is missing or invalid.
- `400 Bad Request` when the body is not one JSON object.
- `422 Unprocessable Entity` when the value has the wrong type, violates a biography rule, or includes an unexpected top-level field.

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

Checks photo metadata only. It does not receive image bytes, upload a file, or save a profile image key. The upload and Tigris integration are not implemented yet.

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

Valid metadata or no photo:

```json
{
  "valid": true
}
```

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
| `photo` | object or `null` | No | Optional photo metadata satisfying the rules above. It is validated but not uploaded or saved. |

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
- `401 Unauthorized` when the Clerk session is missing or invalid.
- `400 Bad Request` when the body is not one JSON object.
- `409 Conflict` when the profile already exists or the username is already in use.
- `422 Unprocessable Entity` when any field is invalid or an unexpected top-level field is present.
- `500 Internal Server Error` when a repository operation fails for another reason.

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
    "message": "Invalid username",
    "field": "username"
  }
}
```

| HTTP status | Error code | Meaning |
| --- | --- | --- |
| `400 Bad Request` | `invalid_request` | The body is not one JSON object. |
| `401 Unauthorized` | `unauthorized` | A valid Clerk session is required. |
| `409 Conflict` | `profile_already_complete` | A local profile already exists for this Clerk user. |
| `409 Conflict` | `username_taken` | The username is already in use. |
| `422 Unprocessable Entity` | `unexpected_field` | An unlisted top-level request field was supplied. |
| `422 Unprocessable Entity` | `invalid_username` | Username is missing, has the wrong type, or violates its format. |
| `422 Unprocessable Entity` | `invalid_gender` | Gender is missing, has the wrong type, or is not an accepted value. |
| `422 Unprocessable Entity` | `invalid_bio` | Biography has the wrong type or violates a biography rule. |
| `422 Unprocessable Entity` | `invalid_photo` | Photo metadata is malformed or violates a photo rule. |
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
5. Submit all profile fields to `POST /api/v1/onboarding/complete`. The server repeats every validation, uses the verified Clerk user ID, checks current username availability, and creates the profile.

The API currently validates photo metadata only. A successful photo validation does not mean an image has been uploaded or stored.
