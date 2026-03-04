# zed-crypt

Transparent encryption for the [Zed](https://zed.dev) editor. Edit `.cpt` files decrypted in Zed while they stay encrypted on disk. Uses [ccrypt](https://ccrypt.sourceforge.net) format — files are fully compatible with the `ccrypt` CLI.

This tool is not intended to secure critical or highly confidential data in case someone gains access to your local filesystem. Instead, it assumes that your local filesystem is already encrypted. The purpose of this tool is to make it easier to edit encrypted files, for example files stored in GitHub repositories.

## Install

```bash
brew tap oglimmer/zed-crypt https://github.com/oglimmer/zed-crypt
brew install zed-crypt
```

This will also install `ccrypt` as a dependency if you don't have it.

## Setup

Create a password file:

```bash
echo "your-password" > ~/.zed-crypt
chmod 600 ~/.zed-crypt
```

## Usage

```bash
# Edit a .cpt file in Zed (decrypt → edit → re-encrypt on save)
zed-crypt edit secret.cpt

# Encrypt a plaintext file → file.cpt
zed-crypt encrypt notes.txt

# Decrypt a .cpt file → plaintext
zed-crypt decrypt notes.cpt
```

### How `edit` works

1. Decrypts the `.cpt` file to a temporary directory
2. Opens the decrypted file in Zed with `zed --wait`
3. Polls for changes every 500ms and re-encrypts back to the `.cpt` file on each save
4. Cleans up the temporary file when Zed closes the buffer or you press Ctrl-C

### New files

Running `zed-crypt edit new-file.cpt` on a file that doesn't exist yet opens an empty buffer. On first save it creates the encrypted `.cpt` file.

## Compatibility

Files created by `zed-crypt` can be decrypted with `ccrypt` and vice versa:

```bash
# Decrypt with ccrypt
ccrypt -d -k ~/.zed-crypt secret.cpt

# Encrypt with ccrypt, edit with zed-crypt
ccrypt -e -k ~/.zed-crypt secret.txt
zed-crypt edit secret.txt.cpt
```
