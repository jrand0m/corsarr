# Homebrew formula for the release binaries (macOS and Linux). Lives in a tap:
#   brew install jrand0m/tap/corsarr && brew services start corsarr
class Corsarr < Formula
  desc "Media automation server in one static binary"
  homepage "https://github.com/jrand0m/corsarr"
  license "AGPL-3.0-or-later"

  on_macos do
    on_arm do
      url "https://github.com/jrand0m/corsarr/releases/download/v0.4.0/corsarr-darwin-arm64.tar.gz"
      sha256 "29c84c51d9347f35c9a46529adf8870125431c43e734d62f53588af23e5d1df0"
    end
    on_intel do
      url "https://github.com/jrand0m/corsarr/releases/download/v0.4.0/corsarr-darwin-amd64.tar.gz"
      sha256 "7e7ba9c15bea1645bdff4f598b1ba5280905a3388347d5774acc945bc8ca49c8"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/jrand0m/corsarr/releases/download/v0.4.0/corsarr-linux-arm64.tar.gz"
      sha256 "83a5bd2545a401408293a7e4865f7f4a114911c2329ab6f38b8e0ede3fafb5c1"
    end
    on_intel do
      url "https://github.com/jrand0m/corsarr/releases/download/v0.4.0/corsarr-linux-amd64.tar.gz"
      sha256 "e8063b0ed93fcec330a220ff5bb0e60963a4af419a3be0098d262b3c4b14169a"
    end
  end

  def install
    bin.install "corsarr"
    # Defaults (as `corsarr config defaults --format=yaml` prints them) with the data
    # directory under Homebrew's var. Homebrew keeps an edited copy on upgrade.
    (buildpath/"corsarr.yml").write <<~YAML
      version: 1
      server:
        address: ":8090"
      data:
        dir: #{var}/corsarr
      log:
        level: info
      auth:
        mode: password
      mediaKinds:
        tv: { mode: active }
        movie: { mode: active }
        music: { mode: active }
        book: { mode: disabled }
    YAML
    (etc/"corsarr").install "corsarr.yml"
  end

  def caveats
    <<~EOS
      Config: #{etc}/corsarr/corsarr.yml
      Data:   #{var}/corsarr
      Web UI: http://localhost:8090
    EOS
  end

  service do
    run [opt_bin/"corsarr", "--configFile=#{etc}/corsarr/corsarr.yml"]
    keep_alive successful_exit: false
    working_dir var/"corsarr"
    log_path var/"log/corsarr.log"
    error_log_path var/"log/corsarr.log"
    environment_variables GOMEMLIMIT: "96MiB"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/corsarr version")
    (testpath/"c.yml").write "version: 1\ndata:\n  dir: #{testpath}\n"
    assert_match "configuration valid", shell_output("#{bin}/corsarr config validate --configFile=#{testpath}/c.yml")
  end
end
