internal sealed class FixtureOutput : IDisposable
{
    private readonly string destination;
    private readonly DirectoryInfo staging;
    private bool published;

    public FixtureOutput(string destination)
    {
        this.destination = Path.GetFullPath(destination);
        if (Path.Exists(this.destination) || new FileInfo(this.destination).LinkTarget is not null)
            throw new IOException("The output directory must not already exist.");
        staging = Directory.CreateTempSubdirectory("gosmos-corpus-");
    }

    public void Write(string name, string content)
    {
        if (name is "" or "." or ".." || Path.GetFileName(name) != name)
            throw new IOException("A fixture name must be a single filename.");
        FileStreamOptions options = new() { Mode = FileMode.CreateNew, Access = FileAccess.Write, Share = FileShare.None };
        if (!OperatingSystem.IsWindows()) options.UnixCreateMode = UnixFileMode.UserRead | UnixFileMode.UserWrite;
        using FileStream file = new(Path.Combine(staging.FullName, name), options);
        using StreamWriter writer = new(file);
        writer.Write(content);
    }

    public void Complete()
    {
        // Publish only a complete private directory; never open files under the caller's path.
        Directory.Move(staging.FullName, destination);
        published = true;
    }

    public void Dispose()
    {
        if (!published) staging.Delete(true);
    }
}
