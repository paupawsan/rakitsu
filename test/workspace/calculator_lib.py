"""
Core calculator logic module.
"""

from typing import Union

class CalculatorError(Exception):
    """Base exception for calculator errors."""
    pass

class DivisionByZeroError(CalculatorError):
    """Raised when division by zero is attempted."""
    pass

def add(a: float, b: float) -> float:
    """Adds two numbers."""
    return a + b

def subtract(a: float, b: float) -> float:
    """Subtracts b from a."""
    return a - b

def multiply(a: float, b: float) -> float:
    """Multiplies two numbers."""
    return a * b

def divide(a: float, b: float) -> float:
    """
    Divides a by b.
    
    Raises:
        DivisionByZeroError: If b is 0.
    """
    if b == 0:
        raise DivisionByZeroError("Cannot divide by zero.")
    return a / b
