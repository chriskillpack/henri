# Henri

<a href="https://en.wikipedia.org/wiki/Henri_Cartier-Bresson"><p align="center"><img src="images/henri_cartier-bresson.jpeg" width="120"></p></a>

An investigation into LLM image search using a multimodal LLM (currently LLaVA) to describe a library of images and then searching those descriptions via embeddings. It's a research project for me, but learn from it if you want. Named after [Henri Cartier-Bresson](https://en.wikipedia.org/wiki/Henri_Cartier-Bresson).

## Usage

```
Usage:
  henri scan, sc <library_path>    Recursively scan library_path for JPEG files
  henri describe, d                Generate textual descriptions for images
  henri embeddings, e              Generate embeddings from image descriptions
  henri query, q <query>           Search embeddings using the query
  henri server, s                  Start webserver (default is port 8080, PORT env var to override)
```

There are command flags which can be used with some of the modes

| **Flag**   | **Description**                                                         | **Default** | **Example**                       |
|------------|-------------------------------------------------------------------------|-------------|-----------------------------------|
| `db`       | Path to the SQLite database. This will be created if it does not exist. | `henri.db`  | `--db foo.db`                     |
| `llama`    | Use the llamafile server running at `http://host:port`.                 | `""`        | `--llama http://localhost:8080`   |
| `seed`     | Seed value to send to llamafile server. Legacy, you can ignore this.    | `385480504` | `--seed 12345678`                 |
| `ollama`   | Use the ollama server running at `http://host:port`.                    | `""`        | `--ollama http://localhost:11434` |
| `openai`   | Use OpenAI API. **Not usable for image description**                    | `false`     | `--openai`                        |
| `count`    | Limit the number of work items to N.                                    | `-1`        | `--count 100`                     |

There is a pipeline of steps that need to be followed in order to get the database populated. These are outlined below in order.

### Step 1 - scan the image library
```
$ go run ./cmd/henri scan ~/Photos/my_photo_library
Found 21397 images on disk
Added 21397 new images
```

### Step 2 - describe the images
Before starting the second step, which is the photo description step, you should make sure your LLM server is running. Either a LLaVA file or ollama. How to start the LLaVA server:

```
(In another terminal window)
$ cd llavafile
$ ./llava-v1.5-7b-q4.llamafile   # Starts a server listening on http://localhost:8080
```

Once your local LLM server is running, start the second step. This is a relatively slow process and may take several days to complete. The longer you leave it running the better, you have more descriptions to search.

```
$ go run ./cmd/henri describe --ollama http://localhost:11434
21397 images to process
Processing 0/21397 <1310: 164E5EBC-F2F5-4B28-A78F-0803857336BE_1_105_c.jpeg> okay, 20 secs
Processing 1/21397 <1311: 16502306-C779-4C32-9E65-5AED005AD9D1_1_105_c.jpeg> okay, 20 secs
Processing 2/21397 <1312: 1650BB2D-5A87-4BDC-A155-9D5BC9BBE8BF_1_105_c.jpeg> okay, 20 secs
Processing 3/21397 <1313: 165AC2AF-E0B4-4EBA-8F94-1B1E03792314_1_105_c.jpeg> okay, 20 secs
Processing 4/21397 <1314: 166BD395-0788-409F-9903-91DFF8B45279_1_105_c.jpeg> okay, 19 secs
Processing 5/21397 <1315: 166BEFB4-EBF0-4C01-83E8-2F1F2DB9C89D_1_105_c.jpeg> okay, 14 secs
Processing 6/21397 <1316: 1675A394-70C3-408D-B63B-05063458483A_1_105_c.jpeg> okay, 20 secs
Processing 7/21397 <1317: 167CD173-33BA-4763-9991-976C42971280_1_105_c.jpeg> okay, 17 secs
Processing 8/21397 <1318: 16812FFB-7428-4260-B6C0-F7E78C69B6E2_1_105_c.jpeg> okay, 14 secs
Processing 9/21397 <1319: 1683DD25-6E33-4754-8C9F-63CB78945DCD_1_105_c.jpeg> okay, 20 secs
Processing 10/21397 <1320: 1684EB8A-3439-47F7-8EFD-F3C5EC9B3536_1_105_c.jpeg> okay, 17 secs
Processing 11/21397 <1321: 16870CC5-21F2-410B-A132-442AA2144E9A_1_105_c.jpeg> okay, 15 secs
Processing 12/21397 <1322: 168ABFBE-4249-4B18-A592-60C331EFC97F_1_105_c.jpeg> okay, 18 secs
Processing 13/21397 <1323: 168B6E18-08B3-467C-92E8-B60A69BF669F_1_102_o.jpeg> okay, 21 secs
Processing 14/21397 <1324: 168B6E18-08B3-467C-92E8-B60A69BF669F_1_105_c.jpeg> okay, 18 secs
Processing 15/21397 <1325: 16933634-DA58-4A8B-9BB9-167E14D7774F_1_105_c.jpeg> okay, 19 secs
Processing 16/21397 <1326: 1694A0F2-D116-4A4E-AF3D-BB40179BB0AC_1_102_o.jpeg> okay, 15 secs
...
```

### Step 3 - compute embedding vectors

Once textual descriptions have been created for all the images the final step is to compute embedding vectors for all the images. Without embeddings the search cannot operate. This is a much quicker process than image description. This is a separate step for legacy reasons, but no reason it cannot happen automatically after image description.

```
$ go run ./cmd/henri embeddings --ollama http://localhost:11434
17623 images to process
Using describer ollama
Processing 0/17623 <416: 06E06DA7-6483-4D91-9ECE-FC99D078C6E0_1_105_c.jpeg> okay, 3 secs
Processing 1/17623 <417: 06E20A31-84AC-48EE-ADB5-7D2568B66F63_1_105_c.jpeg> okay, 0 secs
Processing 2/17623 <418: 06E9E693-0ED3-4FE0-AEC1-221708D0CBCF_1_102_o.jpeg> okay, 0 secs
Processing 3/17623 <419: 06EB7EA8-15EC-4809-A3B6-B7682ED39B4D_1_105_c.jpeg> okay, 0 secs
....
```

## Searching images

```
$ go run ./cmd/henri query "A dog sitting in the sun" --ollama http://localhost:11434
2025/02/11 22:06:17 Checking schema version...
2025/02/11 22:06:17 Schema is up-to-date at digest 3b6b01fbac91682c5be525d99f0cef37cfc57c1909e69099715c2eea34ccde67
Computing query embedding vector...
Computing similarities 100% |████████████████████████████████████████| (9725/9725)
Idx 1    Score=0.15057
Path="/Users/user/Pictures/Photos Library.photoslibrary/resources/derivatives/6/60DEB661-44F3-48CA-8ABC-E6E264CBD4BB_1_105_c.jpeg"
The image shows a smartphone screen displaying an app, likely related to managing tasks or reminders. There are two buttons visible on the screen: one is labeled \"All My Lists,\" and another says \"Update.\" Additionally, there's a button with the word \"Reminder\" next to it. The phone also has some text at the top of the app that reads \"Search today.\""
==========
Idx 2    Score=0.13429
Path="/Users/user/Pictures/Photos Library.photoslibrary/resources/derivatives/4/42004742-2FA7-408C-AB14-B5E5B097E06E_1_105_c.jpeg"
Description="The image shows a white paper with an Amazon return label on it. This document is used to ship items back to the seller after purchase, and includes details such as the order number (148639) and the product being returned: Whirlpool WP11870EMR Refrigerator-Freezer Combination Door Shelf Bin. The return label is also accompanied by a note that reads \"Item received in poor condition.\""
```

First the embedding vector for the query text is computed using the specified LLM. Then all the embedding vectors are searched, scored using cosine similarity, and the top 5 results are shown in decreasing score. The quality of the search results are heavily influenced by the LLM you use. I have seen better search results (from smaller embedding vectors) using OpenAI's text embedding model, than the 7B LLaVA model.

## LLM runners

Henri makes HTTP calls to servers that run LLMs so in theory it can work with any LLM. In practice though each server has different URls or request/response schemas. Currently Henri will work with a llama.cpp webserver such as [llamafile](https://github.com/Mozilla-Ocho/llamafile), [ollama](https://ollama.com/) or the [OpenAI API](https://platform.openai.com/). The OpenAI backend is disabled for image descriptions, due to potential privacy concerns. Sending image descriptions for embedding vector computation and query support is okay though.

### ollama

Now my preferred way of running LLMs, simply because of it's industry support and turnkey installation and model acquisition. Once you are installed ollama.ai make sure you download the llava model as this will be requested directly.

```
ollama pull llava
```

Provide the ollama server

```
go run ./cmd/henri --ollama http://url.to.server:port
```

### llamafile

You will need to install and run the LLaVA model for yourself. For simplicity I used the llamafile implementation, which is a single executable that embeds llama.cpp running as a server and the GGUF model parameters. Model variant I [used](https://huggingface.co/jartine/llava-v1.5-7B-GGUF/blob/main/llava-v1.5-7b-q4.llamafile).

Specify the llamafile server

```
go run ./cmd/henri --llama http://url.to.server:port
```

### OpenAI API

You will need your own OpenAI API key, put the secret key in the environment variable `OPENAI_API_KEY`. Currently henri rate limits queries to OpenAI API's to 20 requests per minute, and it can only be changed in code. The limit and configuration may change in the future.

## Database Migrations

Henri will apply pending database migrations at startup, and only "up" migrations are supported. Migrations are handled by [squibble](https://github.com/tailscale/squibble). The current version of the DB schema is defined in `db/latest_schema.sql`.

## Development

If you are making changes to the web server pages you will need to build new Tailwind CSS. The easiest way is to use the standalone CLI tool, as this eliminates the need to install Node and a lot of packages. See this [blog post](https://tailwindcss.com/blog/standalone-cli) announcing the tool and this more recent [Github Issue](https://github.com/tailwindlabs/tailwindcss/discussions/15855) which is more of a tutorial that covers updated tool behavior.

Assuming the `tailwindcss` CLI tool is in your path, you can generate new CSS using `go generate`:

```
[From the project top level folder]
$ go generate ./cmd/henri
≈ tailwindcss v4.0.7

Done in 47ms
$
```

This will generate new CSS in `cmd/henri/static/tailwind.css` which will need to be committed.

## TODOs

- Thumbnail generation?
