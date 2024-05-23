"""Create a Flask application for LM-Eval-aaS."""
import uuid
import threading
import subprocess
import json
import os
import fnmatch
from flask import Flask, jsonify, request
from flask_cors import CORS

_DEFAULT_USER = 'myuser'
_HOME_PATH = f'/home/{_DEFAULT_USER}'
_OUTPUT_PATH = f'{_HOME_PATH}/output'
_CANCEL_KEY = 'cancel'
_STATUS_KEY = 'status'
_TASK_ID_KEY = 'task_id'
_STATUS_ERROR = 'error'
_STATUS_RUNNING = 'running'
_STATUS_COMPLETE = 'complete'
_STATUS_CANCEALLED = 'cancelled'


def create_app(test_config=None):
    """Create the Flask application."""
    app = Flask(__name__, static_url_path='/doc')
    # enable CORS for the whole application
    CORS(app)

    if test_config:
        # load the test config if passed in
        app.config.from_mapping(test_config)

    # A dictionary to store job status
    # This will be moved to the CRD or storage
    _jobs = {}

    @app.route('/submit_job', methods=['POST'])
    def submit_job():
        """ Submit a job and spawn a subprocess to run the job """
        task_id = str(uuid.uuid4())
        args = [
            ('--model', request.json.get('model')),
            ('--model_args', request.json.get('model_args')),
            ('--tasks', request.json.get('tasks')),
            ('--num_fewshot', None if request.json.get('num_fewshot')
             is None else str(request.json.get('num_fewshot'))),
            ('--gen_kwargs', request.json.get('gen_kwargs')),
            # TODO disable as CLI arg- set to `/output` then return results.json when it's done
            ('--output_path', request.json.get('output_path')),
            # either present or not
            ('--log_samples', request.json.get('log_samples')),
            # needs to be an int or float between 0 and 1
            ('--limit', None if request.json.get('limit')
             is None else str(request.json.get('limit'))),
            ('--use_cache', request.json.get('use_cache')),  # is a path
            ('--decontamination_ngrams_path',
             request.json.get('decontamination_ngrams_path')),
            # present or not
            ('--check_integrity', request.json.get('check_integrity')),
            ('--write_out', request.json.get('write_out')),
            ('--show_config', request.json.get('show_config')),  # present or not
            ('--include_path', request.json.get('include_path'))
        ]

        # Filter out the arguments that are not provided in the request
        novalue_args = ['--check_integrity', '--write_out',
                        '--log_samples', '--trust_remote_code', '--show_config']
        filtered_args = [(arg, value)
                         for arg, value in args if value is not None]
        flat_and_filtered_args = [item if sublist[0] not in novalue_args else sublist[0]
                                  for sublist in filtered_args for item in sublist]

        _jobs[task_id] = {_STATUS_KEY: _STATUS_RUNNING,
                          'args': flat_and_filtered_args}
        threading.Thread(target=_background_task, args=(task_id,)).start()
        return jsonify({_TASK_ID_KEY: task_id})

    @app.route('/poll_job', methods=['GET'])
    def poll_job():
        """ Check the job status by the task_id """
        task_id = request.args.get(_TASK_ID_KEY)
        if task_id in _jobs:
            return jsonify({_STATUS_KEY: _jobs[task_id][_STATUS_KEY]})

        return jsonify({_STATUS_KEY: 'not found'}), 404

    @app.route('/job_results', methods=['GET'])
    def job_results():
        """ Get the results of a job. Return the results as a JSON object """
        task_id = request.args.get(_TASK_ID_KEY)

        def _handle_error():
            return jsonify({_STATUS_ERROR: _jobs[task_id][_STATUS_ERROR]})

        def _handle_running():
            return jsonify({_STATUS_KEY: 'The job is still running'})

        def _handle_cancelled():
            return jsonify({_STATUS_KEY: 'The job was cancelled'})

        def _handle_complete():
            # Get result-xxxxx.json and return its content
            result_json = _find_result(
                'result*.json', f"{_OUTPUT_PATH}/{task_id}")
            if result_json:
                with open(result_json, 'r', encoding='utf-8') as f:
                    result = json.load(f)
                return jsonify(result)

            return jsonify({_STATUS_ERROR: 'Job completed but no result found'})

        handlers = {
            _STATUS_ERROR: _handle_error,
            _STATUS_RUNNING: _handle_running,
            _STATUS_CANCEALLED: _handle_cancelled,
            _STATUS_COMPLETE: _handle_complete,
        }

        if task_id not in _jobs:
            return jsonify({_STATUS_ERROR: 'The specified job does not exist'}), 404

        if _jobs[task_id][_STATUS_KEY] in handlers:
            return handlers[_jobs[task_id][_STATUS_KEY]]()

        return jsonify({_STATUS_ERROR: f"unknown state: {_jobs[task_id][_STATUS_KEY]}"})

    @app.route('/list_jobs', methods=['GET'])
    def list_jobs():
        """ List all jobs """
        return jsonify(
            list(map(
                lambda task_id: {
                    _TASK_ID_KEY: task_id, _STATUS_KEY: _jobs[task_id][_STATUS_KEY]
                }, _jobs.keys())))

    @app.route('/cancel_job', methods=['DELETE'])
    def cancel_job():
        """ Cancel a running job """
        task_id = request.args.get(_TASK_ID_KEY)

        if task_id not in _jobs:
            return jsonify({_STATUS_ERROR: 'The specified job does not exist'}), 404

        _jobs[task_id][_CANCEL_KEY] = True

        return jsonify(
            {_STATUS_KEY: 'Job canceallation initiated. Check job status with poll_job API.'}
        )

    def _background_task(task_id):
        flat_and_filtered_args = _jobs[task_id]['args']
        os.makedirs(f"{_OUTPUT_PATH}/{task_id}")
        cmd = ['python', '-m', 'lm_eval'] + flat_and_filtered_args + \
            ['--output_path', f"{_OUTPUT_PATH}/{task_id}"]

        # create stdout and stderr for the subprocess
        with open(
                f"{_OUTPUT_PATH}/{task_id}/stdout.log", 'w', encoding="utf-8"
            ) as stdout, open(
                f"{_OUTPUT_PATH}/{task_id}/stderr.log", 'w', encoding="utf-8"
        ) as stderr:

            process = subprocess.Popen(
                cmd, stdout=stdout, stderr=stderr, universal_newlines=True)

            while not _jobs[task_id].get(_CANCEL_KEY, False) and process.returncode is None:
                try:
                    # Execute the command and handle cancellation every
                    # 10 seconds
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    pass

            # handle the cancel case before closing the stderr and stdout
            if _jobs[task_id].get(_CANCEL_KEY, False):
                process.kill()
                _jobs[task_id][_STATUS_KEY] = _STATUS_CANCEALLED

        # handle non-cancel case here
        if not _jobs[task_id].get(_CANCEL_KEY, False):
            if process.returncode != 0:
                # If the subprocess returned a non-zero exit status, return the error output
                with open(
                    f"{_OUTPUT_PATH}/{task_id}/stderr.log", 'r', encoding="utf-8"
                ) as err_out:

                    _jobs[task_id][_STATUS_KEY] = _STATUS_ERROR
                    _jobs[task_id][_STATUS_ERROR] = err_out.read()
            else:
                _jobs[task_id][_STATUS_KEY] = _STATUS_COMPLETE

    def _find_result(pattern, path):
        for base, _, files in os.walk(path):
            for name in files:
                if fnmatch.fnmatch(name, pattern):
                    return os.path.join(base, name)
        return None

    return app


if __name__ == '__main__':
    service_app = create_app()
    service_app.run(debug=True, host='0.0.0.0',
                    port=os.getenv("FLASK_PORT", default="8080"))
