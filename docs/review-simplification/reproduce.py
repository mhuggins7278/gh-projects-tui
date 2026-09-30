#!/usr/bin/env python3
"""Validate review prototypes in Go overlays; optionally repeat benchmarks."""
import argparse
import os
import io
import tarfile
import tempfile
from pathlib import Path
import subprocess
import sys

args = argparse.ArgumentParser()
args.add_argument('--bench', action='store_true', help='repeat the three-sample benchmarks')
opts = args.parse_args()
review = Path(__file__).resolve().parent
repo = review.parents[1]
output = Path('/tmp/gh-projects-tui-simplification-review')
env = dict(os.environ, GOCACHE='/tmp/gh-projects-tui-review-go-cache')

def run(label, *arguments):
    result = subprocess.run(['go', *arguments], cwd=baseline, env=env,
                            capture_output=True, text=True)
    (output/(label+'.txt')).write_text(result.stdout+result.stderr)
    print(f'{label}: exit {result.returncode}', flush=True)
    if result.returncode:
        print(result.stdout+result.stderr)
        raise SystemExit(result.returncode)

# Keep the initial review reproducible after the application is refactored.
# Archive only tracked files at the reviewed commit, into a disposable checkout.
output.mkdir(parents=True, exist_ok=True)
archive = subprocess.run(['git', 'archive', 'fbdc836'], cwd=repo,
                         capture_output=True, check=True).stdout
with tempfile.TemporaryDirectory(prefix='baseline-', dir=output) as scratch:
    baseline = Path(scratch)
    with tarfile.open(fileobj=io.BytesIO(archive)) as source:
        source.extractall(baseline, filter='data')
    subprocess.run([sys.executable, str(review/'prepare.py'), str(baseline)], check=True)
    for variant in ['baseline', 'dead', 'quick', 'keys', 'renderer']:
        run(variant+'-tests', 'test', '-overlay='+str(output/(variant+'.json')), './...')
    run('adapter', 'test', '-overlay='+str(output/'adapter.json'), './internal/github',
        '-run', 'TestReviewOpportunity', '-v')
    run('combined-race', 'test', '-race', '-overlay='+str(output/'combined.json'), './...')
    run('combined-vet', 'vet', '-overlay='+str(output/'combined.json'), './...')
    if opts.bench:
        for variant, benchmark in [('baseline', 'BenchmarkReviewOpportunity'),
                                   ('quick', 'BenchmarkReviewOpportunityNavigation'),
                                   ('keys', 'BenchmarkReviewOpportunityNavigation'),
                                   ('renderer', 'BenchmarkReviewOpportunityColdDetail')]:
            run(variant, 'test', '-overlay='+str(output/(variant+'.json')), './internal/ui',
                '-run', 'TestReviewOpportunityDigest', '-v', '-bench', benchmark,
                '-benchtime=10x', '-count=3')
    print(f'Logs and prototype sources: {output}')
