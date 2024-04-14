from __future__ import print_function

import ast
import inspect
import json
import pprint
import sys
from datetime import datetime
import functools
from contextlib import contextmanager
from os.path import basename, realpath
from textwrap import dedent
import traceback
import colorama

import executing
from pygments import highlight
from pygments.formatters import Terminal256Formatter
from pygments.lexers import PythonLexer as PyLexer, Python3Lexer as Py3Lexer

PYTHON2 = sys.version_info[0] == 2

_absent = object()


def bindStaticVariable(name, value):
    def decorator(fn):
        setattr(fn, name, value)
        return fn

    return decorator


@bindStaticVariable("formatter", Terminal256Formatter())
@bindStaticVariable("lexer", PyLexer(ensurenl=False) if PYTHON2 else Py3Lexer(ensurenl=False))
def colorize(s):
    self = colorize
    return highlight(s, self.lexer, self.formatter)


@contextmanager
def supportTerminalColorsInWindows():
    pass
    # Filter and replace ANSI escape sequences on Windows with equivalent Win32
    # API calls. This code does nothing on non-Windows systems.
    colorama.init()
    yield
    colorama.deinit()


def stderrPrint(*args):
    print(*args, file=sys.stderr)


def isLiteral(s):
    try:
        ast.literal_eval(s)
    except Exception:
        return False
    return True


def colorizedStderrPrint(s):
    colored = colorize(s)
    with supportTerminalColorsInWindows():
        stderrPrint(colored)


# Wrapper middleware for Flask SocketIO #####
def flask_middleware(app):
    def wrapper(environ, start_response):
        relevant_info = {
            "request_method": environ.get("REQUEST_METHOD"),
            "path": environ.get("PATH_INFO"),
            "content_type": environ.get("CONTENT_TYPE"),
            "content_length": environ.get("CONTENT_LENGTH", 0),
            "client_ip": environ.get("REMOTE_ADDR"),
            "user_agent": environ.get("HTTP_USER_AGENT"),
        }
        logger("info", "Eventlet Request with and response {}", relevant_info)
        return app(environ, start_response)

    return wrapper


DEFAULT_PREFIX = ""
DEBUG = False
DEFAULT_LINE_WRAP_WIDTH = 70000  # Characters.
DEFAULT_CONTEXT_DELIMITER = "- "
DEFAULT_OUTPUT_FUNCTION = stderrPrint
DEFAULT_ARG_TO_STRING_FUNCTION = pprint.pformat


class NoContextError(OSError):
    """
    Raised when fails to find or access source code that's
    required to parse and analyze.
    """

    infoMessage = (
        "Failed to access the underlying source code for analysis. Was logger() "
        "invoked in a REPL (e.g. from the command line), a frozen application "
        "(e.g. packaged with PyInstaller), or did the underlying source code "
        "change during execution?"
    )


def callOrValue(obj):
    return obj() if callable(obj) else obj


class Source(executing.Source):
    def get_text_with_indentation(self, node):
        result = self.asttokens().get_text(node)
        if "" in result:
            result = " " * node.first_token.start[1] + result
            result = dedent(result)
        result = result.strip()
        return result


def prefixLinesAfterFirst(prefix, s):
    lines = s.splitlines(True)

    for i in range(1, len(lines)):
        lines[i] = prefix + lines[i]

    return "".join(lines)


def indented_lines(prefix, string):
    lines = string.splitlines()
    if not lines:
        return lines

    return [prefix + lines[0]] + [" " * len(prefix) + line for line in lines[1:]]


def format_pair(prefix, arg, value):
    arg_lines = indented_lines(prefix, arg)
    value_prefix = arg_lines[-1] + ": "

    looksLikeAString = value[0] + value[-1] in ["''", '""']
    if looksLikeAString:  # Align the start of multiline strings.
        value = prefixLinesAfterFirst(" ", value)

    value_lines = indented_lines(value_prefix, value)
    lines = arg_lines[:-1] + value_lines
    return "".join(lines)


def singledispatch(func):
    if "singledispatch" not in dir(functools):

        def unsupport_py2(*args, **kwargs):
            raise NotImplementedError("functools.singledispatch is missing in " + sys.version)

        func.register = func.unregister = unsupport_py2
        return func

    func = functools.singledispatch(func)

    # add unregister based on https://stackoverflow.com/a/25951784
    closure = dict(zip(func.register.__code__.co_freevars, func.register.__closure__))
    registry = closure["registry"].cell_contents
    dispatch_cache = closure["dispatch_cache"].cell_contents

    def unregister(cls):
        del registry[cls]
        dispatch_cache.clear()

    func.unregister = unregister
    return func


@singledispatch
def argumentToString(obj):
    # s = DEFAULT_ARG_TO_STRING_FUNCTION(obj)
    # s = s.replace('', '')  # Preserve string newlines in output.
    return obj


class Logger:
    _pairDelimiter = ", "  
    lineWrapWidth = DEFAULT_LINE_WRAP_WIDTH
    contextDelimiter = DEFAULT_CONTEXT_DELIMITER

    def __init__(
        self,
        format="default",
        outputFunction=DEFAULT_OUTPUT_FUNCTION,
        argToStringFunction=argumentToString,
        includeContext=True,
        contextAbsPath=False,
    ):

        # Format can be default, JSON or logfmt, choose the appropriate output function
        self.enabled = True
        self.customOutput = self._getFormatOutput(format)
        self.includeContext = includeContext
        self.outputFunction = outputFunction
        self.argToStringFunction = argToStringFunction
        self.contextAbsPath = contextAbsPath
        self.tracebackStack = False  # TODO: Implement this better and readable
        print("Logger is initialized with format: ", format)

    def _getFormatOutput(self, format):
        formatter_mapping = {
            "default": self._defaultOutput,
            "json": self._jsonOutput,
            "logfmt": None,
        }
        return formatter_mapping.get(format, self._defaultOutput)

    def __call__(self, *args, **kwargs):
        if self.enabled:
            callFrame = inspect.currentframe().f_back
            try:
                out = self._format(callFrame, *args, **kwargs)
            except NoContextError as err:
                out = "Error: " + err.infoMessage
            self.outputFunction(out)

    def format(self, *args, **kwargs):
        callFrame = inspect.currentframe().f_back
        out = self._format(callFrame, *args, **kwargs)
        return out

    def _format(self, callFrame, *args, **kwargs):

        callNode = Source.executing(callFrame).node
        if callNode is None:
            raise NoContextError()

        if self.tracebackStack:
            context = traceback.format_stack()  # self._formatContext(callFrame, callNode)
        else:
            context = ""

        out = self._formatArgs(callFrame, callNode, context, args, **kwargs)

        return out

    def _formatArgs(self, callFrame, callNode, context, args, **kwargs):
        source = Source.for_frame(callFrame)
        sanitizedArgStrs = [source.get_text_with_indentation(arg) for arg in callNode.args]

        pairs = list(zip(sanitizedArgStrs, args))
        if len(pairs) < 2:
            pairs = None
        else:
            pairs = pairs[2:]
        out = self.customOutput(callFrame, context, pairs, args, **kwargs)
        return out

    # TODO: WIP, Implement this better and readable
    def format_call_stack(self, stack_frames):
        """
        Formats a list of traceback frames into a readable string.

        Args:
            stack_frames: A list of traceback frame objects.

        Returns:
            A string representing the formatted call stack.
        """
        formatted_stack = []
        line_text = ""
        for frame in stack_frames:
            # Extract frame information reliably
            try:
                filename, line_number, function_name, line_text = frame
            except ValueError:
                try:  # Attempt extracting only first 3 elements
                    filename, line_number, function_name = frame[:3]
                except ValueError:  # Handle unexpected frame types
                    formatted_stack.append(f"  Unreadable frame: {frame!r}")
                    continue  # Skip to the next frame

            # Handle filename length
            if len(filename) > 40:
                filename = "..." + filename[-40:]

            # Format frame information
            formatted_frame = f'  File "{filename}", line {line_number}, in {function_name}'
            if line_text:
                formatted_frame += f"\n    {line_text.strip()}"
            formatted_stack.append(formatted_frame)

        return "".join(formatted_stack)

    def _defaultOutput(self, callFrame, context, pairs, *args, **kwars):

        prefix = self.customPrefix(callFrame, args, kwars)
        try:
            message = '"' + args[0][1] + '"'  # Attempt to access second item of first arg
        except IndexError:  # Handle case where args is empty or first arg has only 1 item
            message = ""

        pairs = [(arg, val) for arg, val in pairs]
        pairStrs = [res + "=" + str(val) for res, val in pairs]

        # ctx = {"ctx": list(args)}   Convert args to a list to avoid tuple unpacking issues
        extra = {"extra": kwars}
        for key, value in kwars.items():
            extra += f"{key}={value}, "
        free_args = ", ".join(pairStrs)

        allArgsOnOneLine = " " + message + " " + free_args + extra
        multilineArgs = len(allArgsOnOneLine.splitlines()) > 1

        context = self.format_call_stack(context[:-1])
        allPairs = prefix + context + allArgsOnOneLine
        firstLineTooLong = len(allPairs.splitlines()[0]) > self.lineWrapWidth

        # TODO: Debugging, implement pretty printing & full traceback
        if DEBUG:
            if multilineArgs or firstLineTooLong:
                
                if context:
                    lines = [prefix + context] + [
                        format_pair(len(prefix) * " ", arg, value) for arg, value in pairs
                    ]
                
                else:
                    arg_lines = [format_pair("", arg, value) for arg, value in pairs]
                    lines = indented_lines(prefix, "".join(arg_lines))
           
            else:
                lines = [prefix + context + allArgsOnOneLine]

            return "".join(lines)
        return allPairs

    def _jsonOutput(self, callFrame, context, pairs, *args, **kwargs):

        prefix = self.jsonPrefix(callFrame, args, kwargs)
        try:
            message = args[0][1]  # Attempt to access second item of first arg
        except IndexError:  # Handle case where args is empty or first arg has only 1 item
            message = ""

        # Convert arguments and keyword arguments to key-value pairs
        formatted_pairs = {arg: self.argToStringFunction(val) for arg, val in pairs}
        # formatted_pairs.update(kwargs)  # Merge keyword arguments

        data = {
            "message": message,
            "context": formatted_pairs,
            "extra": kwargs,
        }

        prefix.update(data)

        # Concatenate the prefix and JSON-formatted data
        json_string = json.dumps(prefix)

        return json_string

    def _formatContext(self, callFrame, callNode):
        filename, lineNumber, parentFunction = self._getContext(callFrame, callNode)

        if parentFunction != "<module>":
            parentFunction = "%s()" % parentFunction

        context = "[%s:%s in %s" % (filename, lineNumber, parentFunction)
        return context

    def _formatTime(self):
        now = datetime.now()
        formatted = now.strftime("%H:%M:%S.%f")[:-3]
        return " at %s" % formatted

    def _getContext(self, callFrame, callNode):
        lineNumber = callNode.lineno
        frameInfo = inspect.getframeinfo(callFrame)
        parentFunction = frameInfo.function

        filepath = (realpath if self.contextAbsPath else basename)(frameInfo.filename)
        return filepath, lineNumber, parentFunction

    def enable(self):
        self.enabled = True

    def disable(self):
        self.enabled = False

    def configureOutput(
        self,
        outputFunction=_absent,
        argToStringFunction=_absent,
        includeContext=_absent,
        contextAbsPath=_absent,
    ):
        noParameterProvided = all(v is _absent for k, v in locals().items() if k != "self")
        if noParameterProvided:
            raise TypeError("configureOutput() missing at least one argument")

        if outputFunction is not _absent:
            self.outputFunction = outputFunction

        if argToStringFunction is not _absent:
            self.argToStringFunction = argToStringFunction

        if includeContext is not _absent:
            self.includeContext = includeContext

        if contextAbsPath is not _absent:
            self.contextAbsPath = contextAbsPath

    def customPrefix(self, callFrame, args, kwargs):
        log_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        filename = callFrame.f_code.co_filename.split("/")[-1]
        log_level = args[0]  # Assuming level is the first argument

        # Return the formatted message
        return f"[{str.upper(log_level[0][0])}{log_time} {filename}:{callFrame.f_lineno}]"

    def jsonPrefix(self, callFrame, args, kwargs):
        """Formats a log message as a JSON object."""

        log_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        filename = callFrame.f_code.co_filename.split("/")[-1]
        log_level = args[0]  # Assuming level is the first argument

        # Create a dictionary for the JSON object
        data = {
            "level": str.upper(log_level[0]),
            "timestamp": log_time,
            "filename": filename,
            "line_no": callFrame.f_lineno,
        }

        return data


global logger

logger = Logger(format="json", includeContext=True, contextAbsPath=True)
