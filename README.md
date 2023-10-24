# Mex

Mex is a tool for repacking manga archives into a sane directory structure for consumptions by viewers like
[Komga](https://komga.org/), [Kavita](https://www.kavitareader.com/), and many others.

![](img/mex.png)

## Requirements

You must have both `7za` / `7z` and `unrar` installed on your system and included in your system `PATH`.

## Features

*   Process most compressed formats, including nested archives 🌮
*   Detect best quality volumes in the presence of duplicates 🌶️
*   Rename volumes and pages using templates for consistency 🫔
*   Exclude any irrelevant garbage files present in your archives 🥑
*   Output loose images, CBZ archives, or a combination of both 🌯

## Usage

To process an archive just pass it on the command line to Mex:

```
mex my_manga_archive.rar
```

Additional options can be viewed by running with the `-help` flag:

```
Usage: mex <input_path> [<output_dir>]
  -label-book string
    	book name template (default "{{.Name}}")
  -label-page string
    	page name template (default "page_{{.Index}}{{.Ext}}")
  -label-volume string
    	volume name template (default "vol_{{.Index}}")
  -workers int
    	number of simultaneous workers (default 4)
  -zip-book
    	compress book as a cbz archive
  -zip-volume
    	compress volumes as cbz archives (default true)
Templates:
  {{.Index}} - index of current volume or page
  {{.Name}} - original filename and extension
  {{.Ext}} - original extension only
```
