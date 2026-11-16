# Gosmos regression corpus tools

The Go query tests consume vendored JSON fixtures containing structured query inputs, expected SQL, ordered parameter values, and expected validation errors. Normal Go tests require neither a Cosmos account nor .NET.

`tools/sync-corpus` validates fixture metadata and schema before copying cases into a new destination directory:

```sh
go run ./tools/sync-corpus -source /path/to/generated-cases -destination /path/to/new-corpus
```

Schema version 1 includes `name`, `tags`, `input`, `sql`, `params`, and `errorCategory`, plus source metadata. Parameter names are normalized outside quoted strings while preserving repeated references and value order.

The optional C# fixture generator in this directory requires .NET 10 or newer and a reference project supplied through the `UniverseProject` build property. It accepts a source revision and output directory as positional arguments. Run it against a temporary source copy to keep build artifacts out of the reference checkout.
