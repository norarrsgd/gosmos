# Gosmos regression corpus tools

The Go query tests consume vendored JSON fixtures containing structured query inputs, expected SQL, ordered parameter values, and expected validation errors. Normal Go tests require neither a Cosmos account nor .NET.

`tools/sync-corpus` validates fixture metadata and schema before copying cases into a new destination directory:

```sh
go run ./tools/sync-corpus -source /path/to/generated-cases -destination /path/to/new-corpus
```

Schema version 1 includes `name`, `tags`, `input`, `sql`, `params`, and `errorCategory`, plus source metadata. Parameter names are normalized outside quoted strings while preserving repeated references and value order.

The optional C# fixture generator in this directory requires .NET 10 or newer and a reference project supplied through the `UniverseProject` build property. It accepts a source revision and output directory as positional arguments. Run it against a temporary source copy to keep build artifacts out of the reference checkout.

The output directory must not exist, even as an empty directory or symlink. Its parent must already exist and be controlled by the invoking user; do not use a parent that other users can rename or replace. The exporter writes each fixture exclusively into a fresh private staging directory in the current user's temporary directory, then moves the completed directory to the requested destination. Existing outputs are never reused. Temporary-directory configuration must also be trusted. On Unix, the directory is owner-only and files are created with mode `0600`; Windows inherits the user's temporary-directory ACLs.

The staging directory and destination must be on the same filesystem for publication by directory move. Select an output path on the user's temporary filesystem when needed. A failed or aborted export removes its staging directory and leaves existing destination contents untouched.

Run the standalone filesystem regression checks without a Universe reference, Cosmos account, or third-party test packages:

```sh
dotnet run --project tools/UniverseCorpusExporter.Tests
```

Run from the repository root. These optional checks cover symlink targets, existing/raced destinations, exclusive writes, private permissions, and cleanup; normal Go tests still require no .NET installation.
