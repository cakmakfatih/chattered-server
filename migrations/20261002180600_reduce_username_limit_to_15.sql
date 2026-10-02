DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM "users"
        WHERE char_length("username") > 15
    ) THEN
        RAISE EXCEPTION 'Cannot reduce users.username to 15 characters while longer usernames exist';
    END IF;
END
$$;

ALTER TABLE "users"
    DROP CONSTRAINT "chk_users_username_format",
    ALTER COLUMN "username" TYPE varchar(15),
    ADD CONSTRAINT "chk_users_username_format"
        CHECK ("username" ~ '^[a-z][a-z0-9_]{2,14}$');
