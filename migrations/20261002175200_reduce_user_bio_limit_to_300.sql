DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM "users"
        WHERE char_length("bio") > 300
    ) THEN
        RAISE EXCEPTION 'Cannot reduce users.bio to 300 characters while longer biographies exist';
    END IF;
END
$$;

ALTER TABLE "users" ALTER COLUMN "bio" TYPE varchar(300);
