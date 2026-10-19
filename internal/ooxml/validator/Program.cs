using DocumentFormat.OpenXml;
using DocumentFormat.OpenXml.Packaging;
using DocumentFormat.OpenXml.Validation;

int bad = 0;
foreach (var path in args)
{
    try
    {
        using var doc = path.EndsWith(".xlsx") ? (OpenXmlPackage)SpreadsheetDocument.Open(path, false) : path.EndsWith(".pptx") ? PresentationDocument.Open(path, false) : WordprocessingDocument.Open(path, false);
        var v = new OpenXmlValidator(FileFormatVersions.Office2019);
        var errs = v.Validate(doc).ToList();
        Console.WriteLine($"{Path.GetFileName(path)}: {errs.Count} errors");
        foreach (var e in errs.Take(40))
            Console.WriteLine($"  {e.Part?.Uri} {e.Path?.XPath}: {e.Description}");
        bad += errs.Count;
    }
    catch (Exception ex) { Console.WriteLine($"{path}: cannot open: {ex.Message}"); bad++; }
}
return bad > 0 ? 1 : 0;
