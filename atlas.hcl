data "external_schema" "gorm" {
  program = [
    "go",
    "tool",
    "ariga.io/atlas-provider-gorm",
    "load",
    "--path",
    "./internal/database/models",
    "--dialect",
    "postgres",
  ]
}

env "gorm" {
  src = data.external_schema.gorm.url
  dev = "docker://postgres/17/dev?search_path=public"

  migration {
    dir = "file://migrations"
  }

  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }
}

env "local" {
  url = getenv("DATABASE_URL")

  migration {
    dir = "file://migrations"
  }
}
