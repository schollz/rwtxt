
<p align="center">
<img
    src="/static/img/logo.png"
    width="260" border="0" alt="rwtxt">
<br>
<a href="https://travis-ci.org/schollz/rwtxt"><img
src="https://img.shields.io/travis/schollz/rwtxt.svg?style=flat-square"
alt="Build Status"></a> <a
href="https://github.com/schollz/rwtxt/releases/latest"><img
src="https://img.shields.io/badge/version-1.8.0-brightgreen.svg?style=flat-square"
alt="Version"></a> </p>

<p align="center">A cms for absolute minimalists. Try it at <a href="https://rwtxt.com/public">rwtxt.com</a>.</p>

*rwtxt* is an [open-source](https://github.com/schollz/rwtxt) website where you can store any text online for easy sharing and quick recall. In more specific terms, it is a light-weight and fast content management system (CMS) where you write in Markdown with emphasis on reading.

*rwtxt* builds off [cowyo](https://cowyo.com), a similar app I made previously. In improving with *rwtxt* I aimed to avoid [second-system syndrome](https://en.wikipedia.org/wiki/Second-system_effect): I got rid of features I never used in cowyo (self-destruction, encryption, locking), while integrating a useful new feature not available previously: you can create  *domains*. A *domain* is basically a personalized namespace where you can write private/public posts that are searchable. I personally use *rwtxt* to collect and jot notes for work, personal, coding - each which has its own searchable and indexed domain.

*rwtxt* is backed by a single [sqlite3](https://www.sqlite.org/index.html) database, so it's portable and very easy to backup. It's written in Go and all the assets are bundled so you can just download a single binary and start writing. You can also try the online version: [rwtxt.com](https://rwtxt.com/public).

## Usage

**Reading.** You can share *rwtxt* links to read them on another computer. *rwtxt* is organized in *domains* - places where you can privately or publicly store your text. If the *domain* is private, you must be signed in to read, even you have the permalink.

You can easily [create your own domain](https://rwtxt.com/public) in 10 seconds. When you make a new domain it will be private by default, so only you can view/search/edit your own text.

Once you make a domain you will see an option to make your domain *public* so that anyone can view/search it. However, only people with the domain password can edit in your domain - making *rwtxt* useful as a password-protected wiki. (The one exception is the [`/public`](https://rwtxt.com/public) domain, which anyone can edit/view - making *rwtxt* useful as a pastebin).


**Writing.** To write in *rwtxt*, just create a new page and click "Edit", or goto a URL for the thing you want to write about - like [rwtxt.com/public/write-something](https://rwtxt.com/public/write-something). When you write in *rwtxt* you can format your text in [Markdown](https://guides.github.com/features/mastering-markdown/).

In addition, writing triple backtick code blocks:


    ```javascript
    console.log("hello, world");
    ```

produces code highlighted with [prism.js](https://prismjs.com/):

```javascript
console.log("hello, world");
```

**Deleting.** You can easily delete your page. Just erase all the content from it and it will disappear forever within 10 seconds.

## Install

You can easily install and run *rwtxt* on your own computer.

You can download a binary for the [latest release](https://github.com/schollz/rwtxt/releases/latest).

Or you can install from source. First, make sure to [install Go](https://golang.org/dl/). Then clone the repo:


```bash
$ git clone https://github.com/schollz/rwtxt.git
```

Then you can make it with:

```bash
$ make
```

And then run it!

```bash
$ export PATH="${PATH}:${GOPATH}/bin"
$ rwtxt
```

### Docker

You can also easily install and run with Docker. 

```
$ docker pull schollz/rwtxt
```

Then run by using docker

```
$ docker run -v /place/to/store/data:/data -p 8000:8152 schollz/rwtxt
```

In this case `-p 8000:8152` will have rwtxt will be running on port 8000.

## Notice

By using [rwtxt.com](https://rwtxt.com) you agree to the [terms of service](https://rwtxt.com/rwtxt/terms-of-service).

## License

MIT
