CREATE TABLE "users" (
  "id" uuid NOT NULL DEFAULT gen_random_uuid(),
  "clerk_user_id" varchar(255) NOT NULL,
  "username" varchar(30) NOT NULL,
  "profile_image_key" text NULL,
  "show_last_seen" boolean NOT NULL DEFAULT true,
  "last_seen_at" timestamptz NULL,
  "created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY ("id"),
  CONSTRAINT "chk_users_username_format" CHECK (username ~ '^[a-z][a-z0-9_]{2,29}$')
);

CREATE UNIQUE INDEX "ux_users_clerk_user_id" ON "users" ("clerk_user_id");
CREATE UNIQUE INDEX "ux_users_username" ON "users" ("username");

CREATE TABLE "user_presence_sessions" (
  "user_id" uuid NOT NULL,
  "connection_id" varchar(255) NOT NULL,
  "connected_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
  "last_heartbeat_at" timestamptz NOT NULL,
  "expires_at" timestamptz NOT NULL,
  PRIMARY KEY ("user_id", "connection_id"),
  CONSTRAINT "fk_users_presence_sessions" FOREIGN KEY ("user_id") REFERENCES "users" ("id") ON DELETE CASCADE ON UPDATE CASCADE,
  CONSTRAINT "chk_user_presence_sessions_expiry" CHECK ("expires_at" > "last_heartbeat_at")
);

CREATE INDEX "idx_user_presence_sessions_user_expiry" ON "user_presence_sessions" ("user_id", "expires_at");
CREATE INDEX "idx_user_presence_sessions_expiry" ON "user_presence_sessions" ("expires_at");
