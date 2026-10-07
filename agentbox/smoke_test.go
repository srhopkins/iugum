package agentbox_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Opt-in: AGENTBOX_TEST_IMAGE=agentbox:latest go test ./agentbox -v -timeout 5m
// Docker's active context chooses the engine. Only uniquely named disposable
// containers and volumes created by this test are removed.
func TestDesktopRuntime(t *testing.T) {
	image := os.Getenv("AGENTBOX_TEST_IMAGE")
	if image == "" {
		t.Skip("set AGENTBOX_TEST_IMAGE to run the container integration test")
	}
	docker := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	name := fmt.Sprintf("iugum-agentbox-smoke-%d", time.Now().UnixNano())
	volume := name + "-config"
	docker("volume", "create", volume)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "docker", "rm", "-f", name).CombinedOutput(); err != nil {
			t.Errorf("remove test container: %v: %s", err, out)
		}
		if out, err := exec.CommandContext(ctx, "docker", "volume", "rm", volume).CombinedOutput(); err != nil {
			t.Errorf("remove test volume: %v: %s", err, out)
		}
	})
	start := func() {
		docker("run", "-d", "--name", name, "--shm-size", "1g", "--memory", "8g", "--cpus", "3",
			"-e", "PUID=1000", "-e", "PGID=1000", "-v", volume+":/config", image)
		for range 40 {
			if strings.TrimSpace(docker("inspect", "--format", "{{.State.Health.Status}}", name)) == "healthy" {
				return
			}
			time.Sleep(2 * time.Second)
		}
		t.Fatal("desktop did not become healthy:\n" + docker("logs", "--tail", "100", name))
	}
	inside := func(script string) string {
		return docker("exec", "--user", "abc", "-e", "NODE_PATH=/usr/local/lib/node_modules", name, "bash", "-lc", script)
	}
	start()
	t.Log("supervised editor and Selkies healthy")
	inside(`set -eu
for command in claude codex opencode cursor-agent playwriter iugum bd ffmpeg say agentbox-screenshot agentbox-record; do command -v "$command"; done
curl -fsS http://127.0.0.1:9222/json/version
playwriter session new --direct
agentbox-screenshot /config/recordings/editor.png 8080
test -s /config/recordings/editor.png
node -e 'const fs=require("fs");const p="/config/.config/opencode/opencode.json";const c=JSON.parse(fs.readFileSync(p));c.model="smoke/preserved";fs.writeFileSync(p,JSON.stringify(c));fs.writeFileSync("/config/workspace/preserved.txt","keep me");'
`)
	docker("rm", "-f", name)
	start()
	inside(`set -eu
jq -e '.model == "smoke/preserved" and .mcp.playwriter.enabled' /config/.config/opencode/opencode.json
test "$(cat /config/workspace/preserved.txt)" = 'keep me'
`)
	t.Log("settings, browser and project files survived container recreation")
	inside(`node <<'JS'
const {chromium}=require('playwright');
const {spawn,execFileSync}=require('node:child_process');
const fs=require('node:fs');
const wait=ms=>new Promise(r=>setTimeout(r,ms));
(async()=>{
  const browser=await chromium.launch({args:['--no-sandbox','--autoplay-policy=no-user-gesture-required']});
  let recorder;
  try {
    const page=await browser.newPage({ignoreHTTPSErrors:true});
    await page.goto('https://127.0.0.1:3001/');
    const play=page.getByRole('button',{name:'Play Stream',exact:true});
    if(await play.isVisible()) await play.click();
    for(let n=0;n<30&&!fs.existsSync('/config/.selkies-recording');n++) await wait(500);
    if(!fs.existsSync('/config/.selkies-recording')) throw Error('No recording socket');
    recorder=spawn('agentbox-record',['/config/recordings/demo.mkv'],{stdio:['pipe','ignore','pipe']});
    let log=''; recorder.stderr.on('data',b=>log+=b);
    const stopped=new Promise((resolve,reject)=>{
      recorder.on('error',reject);
      recorder.on('exit',(code,signal)=>resolve({code,signal}));
    });
    await wait(4500);
    execFileSync('say',['The portable desktop records this spoken explanation.'],{timeout:15000});
    await wait(2500);
    recorder.kill('SIGINT');
    const result=await stopped;
    if(![0,255].includes(result.code)) throw Error('Recorder failed: '+JSON.stringify(result)+'\n'+log);
    const probe=JSON.parse(execFileSync('ffprobe',['-v','error','-show_streams','-show_format','-of','json','/config/recordings/demo.mkv']));
    if(!probe.streams.some(s=>s.codec_type==='video')||!probe.streams.some(s=>s.codec_type==='audio')) throw Error('Recording lacks video or audio');
    if(Number(probe.format.duration)<5) throw Error('Recording time collapsed: '+probe.format.duration);
    const decoded=execFileSync('ffmpeg',['-v','error','-i','/config/recordings/demo.mkv','-map','0:a','-f','s16le','-'],{maxBuffer:10000000});
    if(!decoded.some(b=>b!==0)) throw Error('Narration was silent');
    execFileSync('ffmpeg',['-v','error','-i','/config/recordings/demo.mkv','-frames:v','1','-y','/config/recordings/frame.png']);
    console.log('Recording verified:',probe.format.duration,'seconds, video, audible audio, decoded frame');
  } finally {
    if(recorder&&recorder.exitCode===null) recorder.kill('SIGINT');
    await browser.close();
  }
})().catch(e=>{console.error(e);process.exitCode=1;});
JS`)
	t.Log("shared stream recording, screenshots and offline speech verified")
}
