"""
CLI interface for the calculator.
"""

import sys
from typing import Callable, Dict
from calculator_lib import add, subtract, multiply, divide, CalculatorError

def get_number(prompt: str) -> float:
    """Prompts user for a numeric input."""
    while True:
        try:
            return float(input(prompt))
        except ValueError:
            print("Invalid input. Please enter a numeric value.")

def run_cli() -> None:
    """Runs the calculator CLI."""
    operations: Dict[str, Callable[[float, float], float]] = {
        '1': add,
        '2': subtract,
        '3': multiply,
        '4': divide
    }

    print("Simple Calculator")
    print("1. Add")
    print("2. Subtract")
    print("3. Multiply")
    print("4. Divide")

    choice = input("Enter choice (1/2/3/4): ")

    if choice not in operations:
        print("Invalid choice")
        return

    try:
        num1 = get_number("Enter first number: ")
        num2 = get_number("Enter second number: ")
        
        result = operations[choice](num1, num2)
        print(f"Result: {result}")
    except CalculatorError as e:
        print(f"Error: {e}")
    except Exception as e:
        print(f"An unexpected error occurred: {e}")

if __name__ == "__main__":
    run_cli()
