# Source build formula (includes systray support).
# For pre-built binaries: brew install nrflow/tap/nrflow
class Nrflow < Formula
  desc "Multi-workflow agent orchestration system"
  homepage "https://github.com/anderfredx/nrflow"
  url "https://github.com/anderfredx/nrflow/archive/refs/tags/v0.1.0.tar.gz"
  sha256 "PLACEHOLDER"
  license "MIT"

  depends_on "go" => :build
  depends_on "node" => :build

  def install
    # Build UI
    cd "ui" do
      system "npm", "install"
      system "npm", "run", "build"
    end

    # Copy UI dist to embed directory
    rm_rf "be/internal/static/dist"
    cp_r "ui/dist", "be/internal/static/dist"

    # Build and install Go binaries
    cd "be" do
      ldflags = %W[
        -s -w
        -X be/internal/cli.version=#{version}
      ]
      system "go", "build", *std_go_args(ldflags:, output: bin/"nrflow"), "./cmd/nrflow"
      system "go", "build", *std_go_args(ldflags:, output: bin/"nrflow_server"), "./cmd/server"
    end
  end

  def post_install
    (var/"nrflow").mkpath
  end

  def caveats
    <<~EOS
      Database is stored at ~/.nrflow/nrflow.data by default.
      Override with NRFLOW_HOME environment variable.

      To start the server:
        nrflow_server serve

      To start as a background service:
        brew services start nrflow
    EOS
  end

  service do
    run [opt_bin/"nrflow_server", "serve"]
    keep_alive true
    log_path var/"log/nrflow/server.log"
    error_log_path var/"log/nrflow/server.log"
    working_dir var/"nrflow"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/nrflow version 2>&1", 0)
  end
end
