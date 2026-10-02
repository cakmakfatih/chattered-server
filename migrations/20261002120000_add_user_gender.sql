ALTER TABLE "users" ADD COLUMN "gender" varchar(6) NOT NULL;
ALTER TABLE "users" ADD CONSTRAINT "chk_users_gender" CHECK (gender IN ('male', 'female', 'other'));
