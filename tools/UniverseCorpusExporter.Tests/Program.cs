DirectoryInfo root = Directory.CreateTempSubdirectory("gosmos-export-tests-");
int failures = 0;
try
{
    Check("fresh output and private permissions", () =>
    {
        string destination = Path.Combine(root.FullName, "fresh");
        using (FixtureOutput output = new(destination))
        {
            output.Write("empty.json", "fixture\n");
            output.Complete();
        }
        Assert(File.ReadAllText(Path.Combine(destination, "empty.json")) == "fixture\n", "fixture changed");
        if (!OperatingSystem.IsWindows())
        {
            Assert(File.GetUnixFileMode(destination) == (UnixFileMode.UserRead | UnixFileMode.UserWrite | UnixFileMode.UserExecute), "directory is not private");
            Assert(File.GetUnixFileMode(Path.Combine(destination, "empty.json")) == (UnixFileMode.UserRead | UnixFileMode.UserWrite), "file is not private");
        }
    });
    Check("existing output symlink cannot overwrite outside file", () =>
    {
        string destination = Path.Combine(root.FullName, "prepopulated");
        Directory.CreateDirectory(destination);
        string victim = Path.Combine(root.FullName, "victim.json");
        File.WriteAllText(victim, "keep");
        File.CreateSymbolicLink(Path.Combine(destination, "empty.json"), victim);
        ExpectIOException(() =>
        {
            using FixtureOutput output = new(destination);
            output.Write("empty.json", "replace");
            output.Complete();
        });
        Assert(File.ReadAllText(victim) == "keep", "outside file overwritten");
    });
    Check("output directory symlink is refused", () =>
    {
        string target = Path.Combine(root.FullName, "target");
        Directory.CreateDirectory(target);
        string destination = Path.Combine(root.FullName, "directory-link");
        Directory.CreateSymbolicLink(destination, target);
        ExpectIOException(() =>
        {
            using FixtureOutput output = new(destination);
            output.Write("empty.json", "replace");
            output.Complete();
        });
        Assert(!File.Exists(Path.Combine(target, "empty.json")), "symlink target modified");
    });
    Check("destination created during export is refused", () =>
    {
        string destination = Path.Combine(root.FullName, "raced");
        using FixtureOutput output = new(destination);
        output.Write("empty.json", "fixture");
        Directory.CreateDirectory(destination);
        File.WriteAllText(Path.Combine(destination, "keep"), "keep");
        ExpectIOException(output.Complete);
        Assert(File.ReadAllText(Path.Combine(destination, "keep")) == "keep", "existing output changed");
    });
    Check("duplicate file cannot overwrite", () =>
    {
        string destination = Path.Combine(root.FullName, "duplicate");
        using FixtureOutput output = new(destination);
        output.Write("empty.json", "first");
        ExpectIOException(() => output.Write("empty.json", "second"));
        output.Complete();
        Assert(File.ReadAllText(Path.Combine(destination, "empty.json")) == "first", "first fixture overwritten");
    });
    Check("fixture paths cannot escape staging", () =>
    {
        using FixtureOutput output = new(Path.Combine(root.FullName, "escape"));
        ExpectIOException(() => output.Write("../escape.json", "replace"));
        Assert(!File.Exists(Path.Combine(root.FullName, "escape.json")), "fixture escaped staging");
    });
    Check("aborted export has no published directory", () =>
    {
        string destination = Path.Combine(root.FullName, "aborted");
        using (FixtureOutput output = new(destination)) output.Write("empty.json", "fixture");
        Assert(!Directory.Exists(destination), "partial export published");
    });
}
finally
{
    root.Delete(true);
}
Console.WriteLine($"Exporter filesystem checks: {7 - failures}/7 passed.");
return failures == 0 ? 0 : 1;

void Check(string name, Action test)
{
    try { test(); }
    catch (Exception error)
    {
        failures++;
        Console.Error.WriteLine($"FAIL {name}: {error.Message}");
    }
}
static void Assert(bool condition, string message)
{
    if (!condition) throw new InvalidOperationException(message);
}
static void ExpectIOException(Action action)
{
    try { action(); }
    catch (IOException) { return; }
    throw new InvalidOperationException("expected IOException");
}
