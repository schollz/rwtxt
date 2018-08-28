
<p align="center">
<img
    src="/static/img/logo.png"
    width="260" border="0" alt="rwtxt">
<br>
<a href="https://travis-ci.org/schollz/rwtxt"><img
src="https://img.shields.io/travis/schollz/rwtxt.svg?style=flat-square"
alt="Build Status"></a> <a
href="https://github.com/schollz/rwtxt/releases/latest"><img
src="https://img.shields.io/badge/version-1.0.0-brightgreen.svg?style=flat-square"
alt="Version"></a> </p>

<p align="center">A cms for absolute minimalists</a></p>

**rwtxt.com** is an [open-source](https://github.com/schollz/rwtxt) website where you can store any text online for easy sharing. In more specific terms, it is a content management system (CMS), but for absolute minimalists.

Previously I made feature-rich CMS for minimalists, [cowyo.com](https://cowyo.com). *rwtxt* integrates a new feature not available previously: you can create personalized and private posts via *domains*.

## Usage 

**Reading.** You can share *rwtxt* links to read them on another computer. *rwtxt* is organized in *domains* - places where you can privately or publicly store your text. If the *domain* is private, you must be signed in to read, even you have the permalink.

You can easily [create your own domain](https://rwtxt.com/public) in 10 seconds. When you make a new domain it will be private by default, so only you can view/search/edit your own text. 

Once you make a domain you will se an option to make your domain *public* so that anyone can view/search it. However, only people with the domain password can edit in your domain - making *rwtxt* useful as a password-protected wiki. (The one exception is the [`/public`](https://rwtxt.com/public) domain, which anyone can edit/view - making *rwtxt* useful as a pastebin).


**Writing.** To write in *rwtxt*, just create a new page and click "Edit", or goto a URL for the thing you want to write about - like `rwtxt.com/something-i-want-to-write`. When you write in *rwtxt* you can format your text in [Markdown](https://guides.github.com/features/mastering-markdown/).

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

You can easily install and run *rwtxt* on your own computer. First make sure to [install Go](https://golang.org/dl/). Then clone the repo:


```bash
$ git clone https://github.com/schollz/rwtxt.git
```

Then you can make it with:

```bash
$ make
```

And then run it!

```bash
$ ./rwtxt
```

## Alternatives

This program, *rwtxt* is based off my other writing program [cowyo](https://cowyo.com). However, *cowyo* was actually based off [shrib.com](https://shrib.com). 

If you are looking for other alternative minimalist microblogging / note-jotting programs, also check out [write.as](https://write.as/), [txti.es](http://txti.es/), or [typegram](https://en.tgr.am/).


## Notice

By using [rwtxt.com](https://rwtxt.com) you agree to the [terms of service](https://rwtxt.com/rwtxt/terms-of-service).

## License

MIT