# Henri

Named after Henri Cartier-Bresson [wiki](https://en.wikipedia.org/wiki/Henri_Cartier-Bresson)

A small simple utility that uses a LLaVA multimodal LLM server to classify a library of photos and stores the descriptions in a SQLite DB. That's it. It's a research project for me, but learn from it if you want.

## Usage

The utility has 3 modes accessible by command line arguments. The first (and initial necessary step) is to scan an image library using the `--library <path_to_library>` command line option. Once scanning is complete the app will terminate.

The two other modes are `--embeddings` and describe (which is implicitly picked if it's not one of the two other modes). The second sends each image to the LLM and asks it to describe the image. The former option computes an embedding vector for each image description.

### Step 1 - scan the image library
```
$ go run ./cmd/henri --library ~/Photos/my_photo_library
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

Once the server is running proceed with the second step. The lack of command line option implies image description mode. This will take a while (potentially days).

```
$ go run ./cmd/henri --ollama http://localhost:11434
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

```
$ go run ./cmd/henri --embeddings --ollama http://localhost:11434
17623 images to process
Using describer ollama
Processing 0/17623 <416: 06E06DA7-6483-4D91-9ECE-FC99D078C6E0_1_105_c.jpeg> okay, 3 secs
Processing 1/17623 <417: 06E20A31-84AC-48EE-ADB5-7D2568B66F63_1_105_c.jpeg> okay, 0 secs
Processing 2/17623 <418: 06E9E693-0ED3-4FE0-AEC1-221708D0CBCF_1_102_o.jpeg> okay, 0 secs
Processing 3/17623 <419: 06EB7EA8-15EC-4809-A3B6-B7682ED39B4D_1_105_c.jpeg> okay, 0 secs
....
```

## LLM runners

Henri makes HTTP calls to servers that run LLMs so in theory it can work with any LLM. In practice though each server has different URls or request/response schemas. Currently Henri will work with a llama.cpp webserver such as [llamafile]((https://github.com/Mozilla-Ocho/llamafile) or [ollama](https://ollama.com/) webserver.

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

## Database Migrations

Henri will apply pending database migrations at startup, and only "up" migrations are supported. Migrations are handled by [squibble](https://github.com/tailscale/squibble). The current version of the DB schema is defined in `db/latest_schema.sql`.

## TODOs

- Query mode
- Thumbnail generation?
