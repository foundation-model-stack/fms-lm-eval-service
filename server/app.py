from flask import Flask, jsonify, request
from flask_cors import CORS
import uuid
import threading
import subprocess
import json

# just for testing API key pass through
import os, fnmatch

app = Flask(__name__, static_url_path='/doc')
CORS(app)

# A dictionary to store job status
jobs = {}

def background_task(task_id):
    flat_and_filtered_args = jobs[task_id]['args']
    os.makedirs(f"/home/myuser/output/{task_id}")
    cmd = ['python', '-m', 'lm_eval'] + flat_and_filtered_args + ['--output_path', f"/home/myuser/output/{task_id}"]

    # create stdout and stderr for the subprocess
    with open(f"/home/myuser/output/{task_id}/stdout.log", 'w') as stdout, open(f"/home/myuser/output/{task_id}/stderr.log", 'w') as stderr:
        process = subprocess.Popen(cmd, stdout=stdout, stderr=stderr, universal_newlines=True)

        while not jobs[task_id].get('cancel', False) and process.returncode is None:
            try:
                # Execute the command and handle cancellation every
                # 10 seconds
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                pass

        # handle the cancel case before closing the stderr and stdout
        if jobs[task_id].get('cancel', False):
            process.kill()
            jobs[task_id]['status'] = 'cancelled'

    # handle non-cancel case here
    if not jobs[task_id].get('cancel', False):
        if process.returncode != 0:
            # If the subprocess returned a non-zero exit status, return the error output
            with open(f"/home/myuser/output/{task_id}/stderr.log", 'r') as err_out:
                jobs[task_id]['status'] = 'error'
                jobs[task_id]['error'] = err_out.read()
        else:
            jobs[task_id]['status'] = 'complete'


@app.route('/submit_job', methods=['POST'])
def submit_job():
    task_id = str(uuid.uuid4())
    args = [
        ('--model', request.json.get('model')),
        ('--model_args', request.json.get('model_args')),
        ('--tasks', request.json.get('tasks')),
        ('--num_fewshot', None if request.json.get('num_fewshot') is None else str(request.json.get('num_fewshot')) ),
        ('--gen_kwargs', request.json.get('gen_kwargs')),
        ('--output_path', request.json.get('output_path')), # TODO disable as CLI arg- set to `/output` then return results.json when it's done
        ('--log_samples', request.json.get('log_samples')), # either present or not
        ('--limit', None if request.json.get('limit') is None else str(request.json.get('limit'))), #needs to be an int or float between 0 and 1
        ('--use_cache', request.json.get('use_cache')), # is a path
        ('--decontamination_ngrams_path', request.json.get('decontamination_ngrams_path')),
        ('--check_integrity', request.json.get('check_integrity')), # present or not
        ('--write_out', request.json.get('write_out')),
        ('--show_config', request.json.get('show_config')), #present or not
        ('--include_path', request.json.get('include_path'))
    ]

    # Filter out the arguments that are not provided in the request
    novalue_args = ['--check_integrity', '--write_out', '--log_samples', '--trust_remote_code', '--show_config']
    filtered_args = [(arg, value) for arg, value in args if value is not None]
    flat_and_filtered_args = [item if sublist[0] not in novalue_args else sublist[0] for sublist in filtered_args for item in sublist]

    jobs[task_id] = {'status': 'running', 'args': flat_and_filtered_args}
    threading.Thread(target=background_task, args=(task_id,)).start()
    return jsonify({'task_id': task_id})

@app.route('/poll_job', methods=['GET'])
def poll_job():
    task_id = request.args.get('task_id')
    if task_id in jobs:
        return jsonify({'status': jobs[task_id]['status']})
    else:
        return jsonify({'status': 'not found'}), 404

@app.route('/job_results', methods=['GET'])
def job_results():
    task_id = request.args.get('task_id')

    if task_id not in jobs:
        return jsonify({'error': 'The specified job does not exist'}), 404

    if 'error' == jobs[task_id]['status']:
        return jsonify({'error': jobs[task_id]['error']})
    elif 'running' == jobs[task_id]['status']:
        return jsonify({'status': 'The job is still running'})
    elif 'cancelled' == jobs[task_id]['status']:
        return jsonify({'status': 'The job was cancelled'})
    elif 'complete' == jobs[task_id]['status']:
        # Get result-xxxxx.json and return its content
        resultJson = findResult('result*.json', f"/home/myuser/output/{task_id}")
        if resultJson:
            with open(resultJson, 'r') as f:
                result = json.load(f)
            return jsonify(result)
        else:
            return jsonify({'error': 'Job completed but no result found'})
    else:
        return jsonify({'error': 'Result not found'}), 404

@app.route('/list_jobs', methods=['GET'])
def list_jobs():
    return jsonify(list(map(lambda task_id: {'task_id': task_id, 'status': jobs[task_id]['status']}, jobs.keys())))

@app.route('/cancel_job', methods=['DELETE'])
def calcel_job():
    task_id = request.args.get('task_id')

    if task_id not in jobs:
        return jsonify({'error': 'The specified job does not exist'}), 404
    
    jobs[task_id]['cancel'] = True

    return jsonify({'status': 'Job canceallation initiated. Check job status with poll_job API.'})
    
def findResult(pattern, path):
    for base, _, files in os.walk(path):
        for name in files:
            if fnmatch.fnmatch(name, pattern):
                return os.path.join(base, name)
    return None


if __name__ == '__main__':
    app.run(debug=True, host='0.0.0.0', port=os.getenv("FLASK_PORT", default="8080"))
