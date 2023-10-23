# Mex

<ruby>Mex<rt>漫画展開ツール</rt></ruby> is a tool for repacking manga archives (RAR, CBZ, etc.) into a sane directory
structure which can be readily consumed by viewers like [Komga](https://komga.org/),
[Kavita](https://www.kavitareader.com/), and others.

![](img/mex.png)

## Requirements

You must have both `7za` / `7z` and `unrar` installed on your system.

## Features

*   Extract most compressed formats, handling nested archives 🌮
*   Select best quality volumes when duplicates exist 🌶️
*   Optionally rename volumes and pages for consistency 🫔
*   Exclude any irrelevant garbage files present 🥑
*   Output loose images or repack to CBZ archives 🌯

## Usage

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
